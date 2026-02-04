import argparse
import json
import math
import socket
import struct
import sys
import time

import pandas as pd

try:
    from py_vollib.black.greeks.analytical import delta, gamma, theta, vega
    from py_vollib.black.implied_volatility import implied_volatility
    HAS_VOLLIB = True
except Exception:
    HAS_VOLLIB = False

REQUEST_TIMEOUT_SECONDS = 2.0
POLL_INTERVAL_SECONDS = 0.5
PARAM_DAYS_IN_YEAR = 365.0
MIN_TTE_DAYS = 0.1
DEFAULT_RISK_FREE_RATE = 0.01


def log(message: str) -> None:
    print(message, file=sys.stderr, flush=True)


def parse_router_addr(raw: str):
    if not raw:
        return None
    value = raw.strip()
    if not value or ":" not in value:
        return None
    host, port_str = value.rsplit(":", 1)
    host = host.strip()
    if not host:
        return None
    try:
        port = int(port_str)
    except ValueError:
        return None
    if port <= 0 or port > 65535:
        return None
    return host, port


def _sanitize_number(value):
    if value is None:
        return None
    try:
        casted = float(value)
    except Exception:
        return None
    if math.isnan(casted) or math.isinf(casted):
        return None
    return casted


def _sanitize_text(value):
    if value is None:
        return None
    try:
        if pd.isna(value):
            return None
    except Exception:
        pass
    text = str(value).strip()
    if not text:
        return None
    return text


def _option_type_to_cp(raw):
    token = str(raw).strip()
    if token == "1":
        return "c"
    if token == "2":
        return "p"
    return None


def _normalize_tte_days(value):
    tte_days = _sanitize_number(value)
    if tte_days is None:
        return None
    if tte_days < 0:
        return None
    return max(float(tte_days), MIN_TTE_DAYS)


def _read_exact(conn: socket.socket, size: int) -> bytes:
    chunks = []
    remaining = size
    while remaining > 0:
        chunk = conn.recv(remaining)
        if not chunk:
            raise EOFError("connection closed")
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


def _rpc_call(router_target, method: str, params: dict, req_id: int):
    payload = {
        "jsonrpc": "2.0",
        "id": req_id,
        "method": method,
        "params": params,
    }
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    frame = struct.pack(">I", len(encoded)) + encoded
    with socket.create_connection(router_target, timeout=REQUEST_TIMEOUT_SECONDS) as conn:
        conn.sendall(frame)
        header = _read_exact(conn, 4)
        body_len = struct.unpack(">I", header)[0]
        raw = _read_exact(conn, body_len)
    response = json.loads(raw.decode("utf-8"))
    if isinstance(response, dict) and response.get("error"):
        raise RuntimeError(str(response.get("error")))
    return response.get("result")


def _rpc_notify(router_target, method: str, params: dict) -> None:
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params,
    }
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    frame = struct.pack(">I", len(encoded)) + encoded
    with socket.create_connection(router_target, timeout=REQUEST_TIMEOUT_SECONDS) as conn:
        conn.sendall(frame)


def _compute_greeks(option_type, underlying, strike, option_price, tte_days, risk_free_rate):
    if not HAS_VOLLIB:
        return None, None, None, None, None
    if option_type not in ("c", "p"):
        return None, None, None, None, None
    if underlying is None or strike is None or option_price is None:
        return None, None, None, None, None
    if underlying <= 0 or strike <= 0 or option_price <= 0:
        return None, None, None, None, None
    tte_days = _normalize_tte_days(tte_days)
    if tte_days is None:
        return None, None, None, None, None
    t_year = tte_days / PARAM_DAYS_IN_YEAR
    try:
        sigma = implied_volatility(option_price, underlying, strike, risk_free_rate, t_year, option_type)
        sigma = _sanitize_number(sigma)
        if sigma is None or sigma <= 0:
            return None, None, None, None, None
        return (
            sigma,
            _sanitize_number(delta(option_type, underlying, strike, t_year, risk_free_rate, sigma)),
            _sanitize_number(gamma(option_type, underlying, strike, t_year, risk_free_rate, sigma)),
            _sanitize_number(theta(option_type, underlying, strike, t_year, risk_free_rate, sigma)),
            _sanitize_number(vega(option_type, underlying, strike, t_year, risk_free_rate, sigma)),
        )
    except Exception:
        return None, None, None, None, None


def build_options_snapshot(rows, risk_free_rate: float) -> dict:
    ts = int(time.time() * 1000)
    if not rows:
        return {"schema_version": 1, "ts": ts, "rows": []}
    df = pd.DataFrame(rows)
    if df.empty or "product_class" not in df.columns:
        return {"schema_version": 1, "ts": ts, "rows": []}

    class_tokens = df["product_class"].astype("string").str.strip()
    options_df = df[class_tokens.fillna("") == "2"].copy()
    if options_df.empty:
        return {"schema_version": 1, "ts": ts, "rows": []}

    under_map = {}
    if "ctp_contract" in df.columns and "last" in df.columns:
        raw_under = df[["ctp_contract", "last"]].copy()
        raw_under["ctp_contract"] = raw_under["ctp_contract"].astype("string")
        under_map = raw_under.set_index("ctp_contract")["last"].to_dict()

    options_df["underlying"] = options_df.get("underlying", pd.Series(dtype="string")).astype("string")
    options_df["underlying_price"] = options_df["underlying"].map(under_map)
    options_df["option_type_cp"] = options_df.get("option_type", pd.Series(dtype="string")).map(_option_type_to_cp)
    options_df["strike"] = pd.to_numeric(options_df.get("strike"), errors="coerce")
    options_df["last"] = pd.to_numeric(options_df.get("last"), errors="coerce")
    options_df["volume"] = pd.to_numeric(options_df.get("volume"), errors="coerce")

    trading_date = pd.to_datetime(options_df.get("trading_date"), errors="coerce")
    expiry_date = pd.to_datetime(options_df.get("expiry_date"), errors="coerce")
    options_df["tte"] = (expiry_date - trading_date).dt.days
    options_df["tte"] = options_df["tte"].clip(lower=0)

    output_rows = []
    for _, row in options_df.iterrows():
        contract = _sanitize_text(row.get("ctp_contract"))
        if contract is None:
            continue
        underlying = _sanitize_text(row.get("underlying"))
        symbol = _sanitize_text(row.get("symbol"))
        strike = _sanitize_number(row.get("strike"))
        underlying_price = _sanitize_number(row.get("underlying_price"))
        option_price = _sanitize_number(row.get("last"))
        tte_days = _normalize_tte_days(row.get("tte"))
        option_type = _sanitize_text(row.get("option_type_cp"))
        iv, d, g, th, v = _compute_greeks(
            option_type=option_type,
            underlying=underlying_price,
            strike=strike,
            option_price=option_price,
            tte_days=tte_days,
            risk_free_rate=risk_free_rate,
        )
        output_rows.append({
            "ctp_contract": contract,
            "underlying": underlying,
            "symbol": symbol,
            "strike": strike,
            "option_type": option_type,
            "last": option_price,
            "volume": _sanitize_number(row.get("volume")),
            "tte": tte_days,
            "underlying_price": underlying_price,
            "iv": iv,
            "delta": d,
            "gamma": g,
            "theta": th,
            "vega": v,
        })
    return {"schema_version": 1, "ts": ts, "rows": output_rows}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="starSling options worker")
    parser.add_argument("--router_addr", required=True, help="Router tcp addr, e.g. 127.0.0.1:19090")
    parser.add_argument("--risk_free_rate", type=float, default=DEFAULT_RISK_FREE_RATE, help="Risk-free rate for option greeks")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    router_target = parse_router_addr(args.router_addr)
    if router_target is None:
        log(f"invalid router_addr: {args.router_addr}")
        return 2
    if not HAS_VOLLIB:
        log("py_vollib unavailable; options worker will publish rows with null greeks")
    last_seq = 0
    req_id = 1
    while True:
        try:
            result = _rpc_call(
                router_target,
                "router.get_latest_market",
                {"min_seq": last_seq},
                req_id,
            )
            req_id += 1
            if not isinstance(result, dict):
                time.sleep(POLL_INTERVAL_SECONDS)
                continue
            if result.get("unchanged"):
                time.sleep(POLL_INTERVAL_SECONDS)
                continue
            seq = result.get("seq")
            if isinstance(seq, (int, float)):
                last_seq = int(seq)
            rows = result.get("rows", [])
            _ = _rpc_call(router_target, "router.get_ui_state", {}, req_id)
            req_id += 1
            options_snapshot = build_options_snapshot(rows, args.risk_free_rate)
            _rpc_notify(router_target, "options.snapshot", options_snapshot)
        except KeyboardInterrupt:
            log("options worker interrupted")
            return 0
        except Exception as exc:
            log(f"options worker loop error: {exc}")
            time.sleep(POLL_INTERVAL_SECONDS)


if __name__ == "__main__":
    raise SystemExit(main())

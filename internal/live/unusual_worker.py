import argparse
import datetime as dt
import json
import math
import socket
import struct
import sys
import time

REQUEST_TIMEOUT_SECONDS = 2.0
POLL_INTERVAL_SECONDS = 0.5
MAX_BACKOFF_SECONDS = 5.0
DEFAULT_CHG_THRESHOLD = 100000.0
DEFAULT_RATIO_THRESHOLD = 0.05
MAX_ITEMS = 80


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


def _safe_float(value):
    if value is None:
        return None
    try:
        casted = float(value)
    except Exception:
        return None
    if math.isnan(casted) or math.isinf(casted):
        return None
    return casted


def _safe_text(value):
    if value is None:
        return ""
    return str(value).strip()


def _option_cp(raw):
    token = str(raw).strip()
    if token == "1":
        return "c"
    if token == "2":
        return "p"
    return token.lower()


def _parse_date(value):
    text = _safe_text(value)
    if not text:
        return None
    token = text
    if " " in token:
        token = token.split(" ", 1)[0]
    if "T" in token:
        token = token.split("T", 1)[0]
    if len(token) == 8 and token.isdigit():
        try:
            return dt.datetime.strptime(token, "%Y%m%d").date()
        except ValueError:
            return None
    for fmt in ("%Y-%m-%d", "%Y/%m/%d"):
        try:
            return dt.datetime.strptime(token, fmt).date()
        except ValueError:
            continue
    return None


def _compute_tte_days(row):
    direct_tte = _safe_float(row.get("tte"))
    if direct_tte is not None:
        return max(0.0, direct_tte)
    expiry = _parse_date(row.get("expiry_date"))
    if expiry is None:
        return None
    trading = _parse_date(row.get("trading_date"))
    if trading is None:
        trading = _parse_date(row.get("datetime"))
    if trading is None:
        return None
    delta_days = (expiry - trading).days
    if delta_days < 0:
        delta_days = 0
    return float(delta_days)


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


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="starSling unusual worker")
    parser.add_argument("--router_addr", required=True, help="Router tcp addr, e.g. 127.0.0.1:19090")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    router_target = parse_router_addr(args.router_addr)
    if router_target is None:
        log(f"invalid router_addr: {args.router_addr}")
        return 2

    last_seq = 0
    req_id = 1
    prev_turnover = {}
    history = []
    backoff_seconds = POLL_INTERVAL_SECONDS

    while True:
        try:
            result = _rpc_call(router_target, "router.get_latest_market", {"min_seq": last_seq}, req_id)
            req_id += 1
            if not isinstance(result, dict):
                time.sleep(POLL_INTERVAL_SECONDS)
                continue
            seq = result.get("seq")
            next_seq = last_seq
            if isinstance(seq, (int, float)):
                next_seq = int(seq)
            if next_seq < last_seq:
                # Router session restarted; drop previous turnover baseline/history.
                prev_turnover = {}
                history = []
                last_seq = next_seq
            if result.get("unchanged"):
                time.sleep(POLL_INTERVAL_SECONDS)
                continue
            rows = result.get("rows", [])

            ui_state = _rpc_call(router_target, "router.get_ui_state", {}, req_id)
            req_id += 1
            if not isinstance(ui_state, dict):
                ui_state = {}
            chg_threshold = _safe_float(ui_state.get("turnover_chg_threshold"))
            ratio_threshold = _safe_float(ui_state.get("turnover_ratio_threshold"))
            if chg_threshold is None or chg_threshold <= 0:
                chg_threshold = DEFAULT_CHG_THRESHOLD
            if ratio_threshold is None or ratio_threshold <= 0:
                ratio_threshold = DEFAULT_RATIO_THRESHOLD

            next_prev_turnover = dict(prev_turnover)
            new_alerts = []
            for row in rows:
                if _safe_text(row.get("product_class")) != "2":
                    continue
                contract = _safe_text(row.get("ctp_contract"))
                if not contract:
                    continue
                turnover = _safe_float(row.get("turnover"))
                if turnover is None:
                    continue
                prev = next_prev_turnover.get(contract)
                next_prev_turnover[contract] = turnover
                if prev is None or prev <= 0:
                    continue
                turnover_chg = turnover - prev
                turnover_ratio = turnover / prev - 1.0
                if turnover_chg < chg_threshold or turnover_ratio < ratio_threshold:
                    continue
                new_alerts.append({
                    "ts": int(time.time() * 1000),
                    "time": _safe_text(row.get("datetime")),
                    "ctp_contract": contract,
                    "symbol": _safe_text(row.get("symbol")),
                    "underlying": _safe_text(row.get("underlying")),
                    "cp": _option_cp(row.get("option_type")),
                    "strike": _safe_float(row.get("strike")),
                    "tte": _compute_tte_days(row),
                    "price": _safe_float(row.get("last")),
                    "volume": _safe_float(row.get("volume")),
                    "turnover": turnover,
                    "turnover_chg": turnover_chg,
                    "turnover_ratio": turnover_ratio,
                })

            next_history = history
            if new_alerts:
                next_history = new_alerts + history
                next_history = next_history[:MAX_ITEMS]
                for alert in new_alerts:
                    _rpc_notify(router_target, "log.append", {
                        "ts": alert["ts"],
                        "level": "WARN",
                        "source": "unusual_worker",
                        "message": (
                            f"unusual {alert['ctp_contract']} "
                            f"chg={alert['turnover_chg']:.0f} ratio={alert['turnover_ratio']:.2%}"
                        ),
                    })

            _rpc_notify(router_target, "unusual.snapshot", {
                "schema_version": 1,
                "ts": int(time.time() * 1000),
                "rows": next_history,
            })

            # Advance local state only after all RPC calls above succeed.
            prev_turnover = next_prev_turnover
            history = next_history
            last_seq = next_seq
            backoff_seconds = POLL_INTERVAL_SECONDS
        except KeyboardInterrupt:
            log("unusual worker interrupted")
            return 0
        except Exception as exc:
            log(f"unusual worker loop error: {exc}")
            time.sleep(backoff_seconds)
            backoff_seconds = min(backoff_seconds * 2, MAX_BACKOFF_SECONDS)


if __name__ == "__main__":
    raise SystemExit(main())

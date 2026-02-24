import argparse
import json
import math
import os
import re
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
MAX_BACKOFF_SECONDS = 5.0
PARAM_DAYS_IN_YEAR = 365.0
MIN_TTE_DAYS = 0.1
DEFAULT_RISK_FREE_RATE = 0.01
HEALTH_LOG_INTERVAL = 30
IV_DEBUG_SAMPLE_LIMIT = 5


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
    token = str(raw).strip().lower()
    if token in ("1", "c", "call", "认购"):
        return "c"
    if token in ("2", "p", "put", "认沽"):
        return "p"
    return None


def _safe_int(value):
    if value is None:
        return None
    try:
        return int(float(value))
    except Exception:
        return None


def _normalize_tte_days(value):
    tte_days = _sanitize_number(value)
    if tte_days is None:
        return None
    if tte_days < 0:
        return None
    return max(float(tte_days), MIN_TTE_DAYS)


def _norm_contract_key(value):
    text = _sanitize_text(value)
    if text is None:
        return None
    return text.lower()


def metadata_cache_dirs() -> list[str]:
    dirs = []
    if sys.platform == "darwin":
        dirs.append(os.path.join(os.path.expanduser("~"), "Library", "Application Support", "starsling", "metadata"))
    else:
        xdg = os.environ.get("XDG_CONFIG_HOME")
        if xdg:
            dirs.append(os.path.join(xdg, "starsling", "metadata"))
        else:
            dirs.append(os.path.join(os.path.expanduser("~"), ".config", "starsling", "metadata"))
    dirs.append(os.path.join(os.getcwd(), "runtime", "metadata"))
    return dirs


def load_metadata_payload(name: str):
    filename = f"{name}.json"
    for base in metadata_cache_dirs():
        path = os.path.join(base, filename)
        try:
            with open(path, "r", encoding="utf-8") as handle:
                cached = json.load(handle)
            return cached.get("data")
        except FileNotFoundError:
            continue
        except Exception as exc:
            log(f"metadata load failed ({name}): {exc}")
            continue
    return None


def _parse_contract_metadata_rows(payload):
    if payload is None:
        return []
    if isinstance(payload, list):
        return payload
    if isinstance(payload, dict):
        data = payload.get("data")
        if isinstance(data, list):
            return data
        if isinstance(data, dict):
            nested = data.get("data")
            if isinstance(nested, list):
                return nested
    return []


class _ContractMetadataCache:
    def __init__(self):
        self._by_contract = {}
        self._option_root_to_underlying_symbol = {}

    def load(self):
        payload = load_metadata_payload("contract")
        rows = _parse_contract_metadata_rows(payload)
        next_map = {}
        for row in rows:
            if not isinstance(row, dict):
                continue
            contract = _sanitize_text(row.get("InstrumentID"))
            if contract is None:
                continue
            raw_symbol = _sanitize_text(row.get("ProductID"))
            product_class = _sanitize_text(row.get("ProductClass"))
            option_type_cp = _option_type_to_cp(row.get("OptionsType"))
            next_map[_norm_contract_key(contract)] = {
                "symbol": None,
                "raw_symbol": raw_symbol,
                "underlying": _sanitize_text(row.get("UnderlyingInstrID")),
                "product_class": product_class,
                "option_type_cp": option_type_cp,
                "contract": contract,
            }
        for key, row in next_map.items():
            if row.get("product_class") == "2":
                continue
            symbol = _normalize_product_symbol(row.get("raw_symbol"))
            if symbol is None:
                symbol = _contract_root(row.get("contract"))
            row["symbol"] = symbol
            next_map[key] = row
        option_root_to_underlying_symbol = {}
        for key, row in next_map.items():
            if row.get("product_class") != "2":
                continue
            symbol = None
            underlying = row.get("underlying")
            if underlying is not None:
                under_row = next_map.get(_norm_contract_key(underlying))
                if under_row is not None:
                    symbol = _sanitize_text(under_row.get("symbol"))
            if symbol is None:
                symbol = _normalize_option_product_symbol(row.get("raw_symbol"), row.get("option_type_cp"))
            if symbol is None:
                symbol = _contract_root(row.get("contract"))
            row["symbol"] = symbol
            next_map[key] = row
            root = _contract_root(row.get("contract"))
            if root is not None:
                root_key = root.lower()
                if root_key not in option_root_to_underlying_symbol and symbol is not None:
                    option_root_to_underlying_symbol[root_key] = symbol
        self._by_contract = next_map
        self._option_root_to_underlying_symbol = option_root_to_underlying_symbol
        if not next_map:
            log("contract metadata mappings unavailable in options worker; fallback inference only")

    def _get(self, contract):
        key = _norm_contract_key(contract)
        if key is None:
            return None
        return self._by_contract.get(key)

    def resolve_contract_symbol(self, contract):
        row = self._get(contract)
        if row is None:
            return None
        return row.get("symbol")

    def resolve_option_underlying(self, contract):
        row = self._get(contract)
        if row is None:
            return None
        if row.get("product_class") != "2":
            return None
        return row.get("underlying")

    def resolve_option_type_cp(self, contract):
        row = self._get(contract)
        if row is None:
            return None
        return row.get("option_type_cp")

    def infer_contract_symbol(self, contract):
        root = _contract_root(contract)
        if root is None:
            return None
        mapped = self._option_root_to_underlying_symbol.get(root.lower())
        if mapped is not None:
            return mapped
        return root

    def infer_option_underlying(self, contract):
        underlying = _infer_option_underlying_from_contract(contract)
        if underlying is None:
            return None
        root = _contract_root(contract)
        if root is None:
            return underlying
        symbol = self._option_root_to_underlying_symbol.get(root.lower())
        if symbol is None:
            return underlying
        replaced = _replace_contract_root(underlying, symbol)
        if replaced is not None:
            return replaced
        return underlying

    def infer_option_type_cp(self, contract):
        return _infer_option_type_from_contract(contract)


def _option_contract_cp_index(contract):
    token = _sanitize_text(contract)
    if token is None:
        return -1
    upper = token.upper()
    if len(upper) < 3:
        return -1
    idx = upper.rfind("-C-")
    if idx > 0:
        return idx + 1
    idx = upper.rfind("-P-")
    if idx > 0:
        return idx + 1
    for i in range(len(upper) - 2, 0, -1):
        ch = upper[i]
        if ch not in ("C", "P"):
            continue
        suffix = upper[i + 1:]
        if suffix == "":
            continue
        all_digits = True
        for c in suffix:
            if c < "0" or c > "9":
                all_digits = False
                break
        if not all_digits:
            continue
        prefix = upper[:i]
        has_digit = False
        for c in prefix:
            if "0" <= c <= "9":
                has_digit = True
                break
        if not has_digit:
            continue
        return i
    return -1


def _infer_option_type_from_contract(contract):
    token = _sanitize_text(contract)
    if token is None:
        return None
    idx = _option_contract_cp_index(token)
    if idx < 0:
        return None
    return "c" if token.upper()[idx] == "C" else "p"


def _infer_option_underlying_from_contract(contract):
    token = _sanitize_text(contract)
    if token is None:
        return None
    idx = _option_contract_cp_index(token)
    if idx <= 0:
        return None
    underlying = token[:idx].strip().rstrip("-_")
    head = _leading_contract_token(underlying)
    if head is not None:
        return head
    return underlying


def _leading_contract_token(contract):
    token = _sanitize_text(contract)
    if token is None:
        return None
    idx = 0
    while idx < len(token) and token[idx].isalpha():
        idx += 1
    if idx == 0:
        return None
    digit_start = idx
    while idx < len(token) and token[idx].isdigit():
        idx += 1
    if idx == digit_start:
        return None
    return token[:idx]


def _normalize_product_symbol(symbol):
    token = _sanitize_text(symbol)
    if token is None:
        return None
    if "_" in token:
        token = token.split("_", 1)[0]
    token = token.strip()
    if token == "":
        return None
    return token


def _normalize_option_product_symbol(symbol, option_cp):
    base = _normalize_product_symbol(symbol)
    if base is None:
        return None
    cp = _sanitize_text(option_cp)
    if cp is not None:
        cp = cp.lower()
    if cp == "c" and len(base) > 1 and base[-1].lower() == "c":
        base = base[:-1]
    if cp == "p" and len(base) > 1 and base[-1].lower() == "p":
        base = base[:-1]
    base = base.strip()
    if base == "":
        return None
    return base


def _replace_contract_root(contract, symbol):
    token = _sanitize_text(contract)
    symbol = _sanitize_text(symbol)
    if token is None or symbol is None:
        return None
    root = _contract_root(token)
    if root is None:
        return None
    return symbol + token[len(root):]

CONTRACT_METADATA_CACHE = _ContractMetadataCache()


def _resolve_option_underlying(contract, existing_underlying):
    mapped = CONTRACT_METADATA_CACHE.resolve_option_underlying(contract)
    if mapped is not None:
        return mapped
    fallback = _sanitize_text(existing_underlying)
    if fallback is not None:
        return fallback
    inferred = CONTRACT_METADATA_CACHE.infer_option_underlying(contract)
    if inferred is not None:
        return inferred
    return None


def _resolve_contract_symbol(contract, existing_symbol):
    mapped = CONTRACT_METADATA_CACHE.resolve_contract_symbol(contract)
    if mapped is not None:
        return mapped
    fallback = _sanitize_text(existing_symbol)
    if fallback is not None:
        return fallback
    inferred = CONTRACT_METADATA_CACHE.infer_contract_symbol(contract)
    if inferred is not None:
        return inferred
    return None


def _resolve_option_type_cp(contract, raw_option_type):
    mapped = CONTRACT_METADATA_CACHE.resolve_option_type_cp(contract)
    if mapped is not None:
        return mapped
    inferred = CONTRACT_METADATA_CACHE.infer_option_type_cp(contract)
    if inferred is not None:
        return inferred
    fallback = _option_type_to_cp(raw_option_type)
    if fallback is not None:
        return fallback
    return None


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


def _safe_append_log(router_target, level: str, source: str, message: str) -> None:
    if not message:
        return
    try:
        _rpc_notify(router_target, "log.append", {
            "ts": int(time.time() * 1000),
            "level": level,
            "source": source,
            "message": message,
        })
    except Exception:
        pass


def _compute_greeks(option_type, underlying, strike, option_price, tte_days, risk_free_rate):
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
    if not HAS_VOLLIB:
        sigma = _fallback_implied_volatility(
            option_type=option_type,
            option_price=option_price,
            underlying=underlying,
            strike=strike,
            risk_free_rate=risk_free_rate,
            t_year=t_year,
        )
        sigma = _sanitize_number(sigma)
        if sigma is None or sigma <= 0:
            return None, None, None, None, None
        d = _sanitize_number(_fallback_delta(option_type, underlying, strike, risk_free_rate, t_year, sigma))
        g = _sanitize_number(_fallback_gamma(underlying, risk_free_rate, t_year, sigma, strike))
        v = _sanitize_number(_fallback_vega(underlying, risk_free_rate, t_year, sigma, strike))
        return sigma, d, g, None, v
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


SQRT_2 = math.sqrt(2.0)
SQRT_2PI = math.sqrt(2.0 * math.pi)


def _norm_cdf(x):
    return 0.5 * (1.0 + math.erf(x / SQRT_2))


def _norm_pdf(x):
    return math.exp(-0.5 * x * x) / SQRT_2PI


def _black_price(option_type, underlying, strike, risk_free_rate, t_year, sigma):
    if sigma <= 0 or t_year <= 0:
        disc = math.exp(-risk_free_rate * t_year)
        intrinsic = max(underlying - strike, 0.0) if option_type == "c" else max(strike - underlying, 0.0)
        return disc * intrinsic
    sqrt_t = math.sqrt(t_year)
    d1 = (math.log(underlying / strike) + 0.5 * sigma * sigma * t_year) / (sigma * sqrt_t)
    d2 = d1 - sigma * sqrt_t
    disc = math.exp(-risk_free_rate * t_year)
    if option_type == "c":
        return disc * (underlying * _norm_cdf(d1) - strike * _norm_cdf(d2))
    return disc * (strike * _norm_cdf(-d2) - underlying * _norm_cdf(-d1))


def _fallback_implied_volatility(option_type, option_price, underlying, strike, risk_free_rate, t_year):
    if option_type not in ("c", "p") or option_price <= 0 or underlying <= 0 or strike <= 0 or t_year <= 0:
        return None
    low = 1e-6
    high = 3.0
    price_low = _black_price(option_type, underlying, strike, risk_free_rate, t_year, low)
    if option_price < price_low:
        return None
    price_high = _black_price(option_type, underlying, strike, risk_free_rate, t_year, high)
    attempts = 0
    while option_price > price_high and high < 20.0 and attempts < 8:
        high *= 2.0
        price_high = _black_price(option_type, underlying, strike, risk_free_rate, t_year, high)
        attempts += 1
    if option_price > price_high:
        return None
    for _ in range(80):
        mid = 0.5 * (low + high)
        price_mid = _black_price(option_type, underlying, strike, risk_free_rate, t_year, mid)
        if abs(price_mid - option_price) < 1e-8:
            return mid
        if price_mid < option_price:
            low = mid
        else:
            high = mid
    return 0.5 * (low + high)


def _fallback_delta(option_type, underlying, strike, risk_free_rate, t_year, sigma):
    if sigma <= 0 or t_year <= 0 or underlying <= 0 or strike <= 0:
        return None
    sqrt_t = math.sqrt(t_year)
    d1 = (math.log(underlying / strike) + 0.5 * sigma * sigma * t_year) / (sigma * sqrt_t)
    disc = math.exp(-risk_free_rate * t_year)
    if option_type == "c":
        return disc * _norm_cdf(d1)
    return -disc * _norm_cdf(-d1)


def _fallback_gamma(underlying, risk_free_rate, t_year, sigma, strike):
    if sigma <= 0 or t_year <= 0 or underlying <= 0 or strike <= 0:
        return None
    sqrt_t = math.sqrt(t_year)
    d1 = (math.log(underlying / strike) + 0.5 * sigma * sigma * t_year) / (sigma * sqrt_t)
    disc = math.exp(-risk_free_rate * t_year)
    return disc * _norm_pdf(d1) / (underlying * sigma * sqrt_t)


def _fallback_vega(underlying, risk_free_rate, t_year, sigma, strike):
    if sigma <= 0 or t_year <= 0 or underlying <= 0 or strike <= 0:
        return None
    sqrt_t = math.sqrt(t_year)
    d1 = (math.log(underlying / strike) + 0.5 * sigma * sigma * t_year) / (sigma * sqrt_t)
    disc = math.exp(-risk_free_rate * t_year)
    return underlying * disc * _norm_pdf(d1) * sqrt_t


def _compute_vwap(bid1, ask1, bid_vol1, ask_vol1):
    bid_price = _sanitize_number(bid1)
    ask_price = _sanitize_number(ask1)
    bid_vol = _safe_int(bid_vol1)
    ask_vol = _safe_int(ask_vol1)
    if bid_price is None or ask_price is None:
        return None
    if bid_vol is None or ask_vol is None:
        return None
    total_vol = bid_vol + ask_vol
    if total_vol <= 0:
        return None
    return (bid_price * bid_vol + ask_price * ask_vol) / float(total_vol)


def _compute_mid(bid1, ask1):
    bid_price = _sanitize_number(bid1)
    ask_price = _sanitize_number(ask1)
    if bid_price is None or ask_price is None:
        return None
    if bid_price <= 0 or ask_price <= 0:
        return None
    return 0.5 * (bid_price + ask_price)


def _compute_greeks_with_price_fallback(
    option_type,
    underlying,
    strike,
    tte_days,
    risk_free_rate,
    vwap_price,
    mid_price,
    last_price,
):
    price_candidates = []

    def _append_price_candidate(source, raw_price):
        price = _sanitize_number(raw_price)
        if price is None or price <= 0:
            return
        price_candidates.append((source, price))

    _append_price_candidate("vwap", vwap_price)
    _append_price_candidate("mid", mid_price)
    _append_price_candidate("last", last_price)

    if not price_candidates:
        reason = _iv_input_reason(option_type, underlying, strike, None, tte_days)
        return None, None, None, None, None, None, None, reason

    final_reason = "solver_failed"
    selected_price = price_candidates[-1][1]
    selected_source = price_candidates[-1][0]
    for source, option_price in price_candidates:
        selected_price = option_price
        selected_source = source
        iv_reason = _iv_input_reason(option_type, underlying, strike, option_price, tte_days)
        iv, d, g, th, v = _compute_greeks(
            option_type=option_type,
            underlying=underlying,
            strike=strike,
            option_price=option_price,
            tte_days=tte_days,
            risk_free_rate=risk_free_rate,
        )
        if iv is not None:
            return iv, d, g, th, v, selected_price, selected_source, "ok"
        if not HAS_VOLLIB and iv_reason == "ok":
            final_reason = "fallback_solver_failed"
        else:
            final_reason = iv_reason if iv_reason != "ok" else "solver_failed"

    return None, None, None, None, None, selected_price, selected_source, final_reason


def _iv_input_reason(option_type, underlying, strike, option_price, tte_days):
    if option_type not in ("c", "p"):
        return "invalid_option_type"
    if underlying is None or underlying <= 0:
        return "invalid_underlying"
    if strike is None or strike <= 0:
        return "invalid_strike"
    if option_price is None or option_price <= 0:
        return "invalid_option_price"
    if tte_days is None or tte_days <= 0:
        return "invalid_tte"
    return "ok"


def _fmt_debug_number(value):
    num = _sanitize_number(value)
    if num is None:
        return "None"
    return f"{num:.6g}"


def _norm_token(value):
    text = _sanitize_text(value)
    if text is None:
        return None
    return text.lower()


def _contract_root(contract):
    token = _norm_token(contract)
    if token is None:
        return None
    matched = re.match(r"^[a-z]+", token)
    if matched is None:
        return None
    return matched.group(0)


CONTRACT_METADATA_CACHE.load()


def _option_row_matches_focus(row, focus_symbol):
    focus = _norm_token(focus_symbol)
    if focus is None:
        return False
    contract = _norm_token(row.get("ctp_contract"))
    underlying = _norm_token(_resolve_option_underlying(row.get("ctp_contract"), row.get("underlying")))
    symbol = _norm_token(_resolve_contract_symbol(row.get("ctp_contract"), row.get("symbol")))
    return contract == focus or underlying == focus or symbol == focus


def _mean_iv_in_abs_delta_band(chain_df, delta_min, delta_max):
    if chain_df is None or chain_df.empty:
        return None
    filtered = chain_df[(chain_df["delta"].abs() >= delta_min) & (chain_df["delta"].abs() <= delta_max)]
    if filtered.empty:
        return None
    return _sanitize_number(filtered["iv"].mean())


def _compute_side_skew(chain_df):
    if chain_df is None or chain_df.empty:
        return None
    atm_iv = _mean_iv_in_abs_delta_band(chain_df, 0.45, 0.55)
    iv25 = _mean_iv_in_abs_delta_band(chain_df, 0.2, 0.3)
    if atm_iv is None or iv25 is None:
        return None
    return _sanitize_number(iv25 - atm_iv)


def build_options_snapshot(rows, risk_free_rate: float):
    ts = int(time.time() * 1000)
    if not rows:
        return {"schema_version": 1, "ts": ts, "rows": []}, {
            "options_total": 0,
            "iv_valid": 0,
            "vwap_valid": 0,
            "last_fallback_used": 0,
            "price_valid": 0,
            "underlying_valid": 0,
            "iv_failure_total": 0,
            "iv_failures": {},
            "iv_debug_samples": [],
        }
    df = pd.DataFrame(rows)
    if df.empty or "product_class" not in df.columns:
        return {"schema_version": 1, "ts": ts, "rows": []}, {
            "options_total": 0,
            "iv_valid": 0,
            "vwap_valid": 0,
            "last_fallback_used": 0,
            "price_valid": 0,
            "underlying_valid": 0,
            "iv_failure_total": 0,
            "iv_failures": {},
            "iv_debug_samples": [],
        }

    class_tokens = df["product_class"].astype("string").str.strip()
    options_df = df[class_tokens.fillna("") == "2"].copy()
    if options_df.empty:
        return {"schema_version": 1, "ts": ts, "rows": []}, {
            "options_total": 0,
            "iv_valid": 0,
            "vwap_valid": 0,
            "last_fallback_used": 0,
            "price_valid": 0,
            "underlying_valid": 0,
            "iv_failure_total": 0,
            "iv_failures": {},
            "iv_debug_samples": [],
        }

    under_map = {}
    if "ctp_contract" in df.columns and "last" in df.columns:
        raw_under = df[["ctp_contract", "last"]].copy()
        raw_under["contract_norm"] = raw_under["ctp_contract"].astype("string").str.strip().str.lower()
        raw_under["last"] = pd.to_numeric(raw_under["last"], errors="coerce")
        raw_under = raw_under.dropna(subset=["contract_norm", "last"])
        under_map = raw_under.set_index("contract_norm")["last"].to_dict()

    options_df["ctp_contract"] = options_df.get("ctp_contract", pd.Series(dtype="string")).astype("string")
    options_df["underlying"] = options_df.apply(
        lambda row: _resolve_option_underlying(row.get("ctp_contract"), row.get("underlying")),
        axis=1,
    )
    options_df["symbol"] = options_df.apply(
        lambda row: _resolve_contract_symbol(row.get("ctp_contract"), row.get("symbol")),
        axis=1,
    )
    options_df["option_type_cp"] = options_df.apply(
        lambda row: _resolve_option_type_cp(row.get("ctp_contract"), row.get("option_type")),
        axis=1,
    )
    options_df["underlying_norm"] = options_df["underlying"].astype("string").str.strip().str.lower()
    options_df["underlying_price"] = options_df["underlying_norm"].map(under_map)
    options_df["strike"] = pd.to_numeric(options_df.get("strike"), errors="coerce")
    options_df["last"] = pd.to_numeric(options_df.get("last"), errors="coerce")
    options_df["volume"] = pd.to_numeric(options_df.get("volume"), errors="coerce")
    options_df["bid1"] = pd.to_numeric(options_df.get("bid1"), errors="coerce")
    options_df["ask1"] = pd.to_numeric(options_df.get("ask1"), errors="coerce")
    options_df["bid_vol1"] = pd.to_numeric(options_df.get("bid_vol1"), errors="coerce")
    options_df["ask_vol1"] = pd.to_numeric(options_df.get("ask_vol1"), errors="coerce")

    trading_date = pd.to_datetime(options_df.get("trading_date"), errors="coerce")
    expiry_date = pd.to_datetime(options_df.get("expiry_date"), errors="coerce")
    options_df["tte"] = (expiry_date - trading_date).dt.days + 1
    options_df["tte"] = options_df["tte"].clip(lower=0)

    output_rows = []
    diagnostics = {
        "options_total": int(len(options_df)),
        "iv_valid": 0,
        "vwap_valid": 0,
        "last_fallback_used": 0,
        "price_valid": 0,
        "underlying_valid": 0,
        "iv_failure_total": 0,
        "iv_failures": {},
        "iv_debug_samples": [],
    }
    for _, row in options_df.iterrows():
        contract = _sanitize_text(row.get("ctp_contract"))
        if contract is None:
            continue
        underlying = _sanitize_text(row.get("underlying"))
        symbol = _sanitize_text(row.get("symbol"))
        strike = _sanitize_number(row.get("strike"))
        underlying_price = _sanitize_number(row.get("underlying_price"))
        option_type_raw = _sanitize_text(row.get("option_type"))
        vwap_price = _compute_vwap(
            row.get("bid1"),
            row.get("ask1"),
            row.get("bid_vol1"),
            row.get("ask_vol1"),
        )
        mid_price = _compute_mid(
            row.get("bid1"),
            row.get("ask1"),
        )
        last_price = _sanitize_number(row.get("last"))
        if vwap_price is not None:
            diagnostics["vwap_valid"] += 1
        if vwap_price is not None or mid_price is not None or last_price is not None:
            diagnostics["price_valid"] += 1
        if underlying_price is not None and underlying_price > 0:
            diagnostics["underlying_valid"] += 1
        tte_days = _normalize_tte_days(row.get("tte"))
        option_type = _sanitize_text(row.get("option_type_cp"))
        iv, d, g, th, v, option_price, price_source, final_iv_reason = _compute_greeks_with_price_fallback(
            option_type=option_type,
            underlying=underlying_price,
            strike=strike,
            tte_days=tte_days,
            risk_free_rate=risk_free_rate,
            vwap_price=vwap_price,
            mid_price=mid_price,
            last_price=last_price,
        )
        if price_source == "last":
            diagnostics["last_fallback_used"] += 1
        if iv is not None:
            diagnostics["iv_valid"] += 1
        else:
            diagnostics["iv_failure_total"] += 1
            diagnostics["iv_failures"][final_iv_reason] = diagnostics["iv_failures"].get(final_iv_reason, 0) + 1
        if len(diagnostics["iv_debug_samples"]) < IV_DEBUG_SAMPLE_LIMIT:
            diagnostics["iv_debug_samples"].append({
                "ctp_contract": contract,
                "option_type_raw": option_type_raw,
                "option_type_cp": option_type,
                "underlying_price": underlying_price,
                "strike": strike,
                "price_for_iv": option_price,
                "price_source": price_source,
                "tte_days": tte_days,
                "iv": iv,
                "reason": final_iv_reason,
            })
        output_rows.append({
            "ctp_contract": contract,
            "underlying": underlying,
            "symbol": symbol,
            "strike": strike,
            "option_type": option_type,
            "option_type_raw": option_type_raw,
            "last": option_price,
            "volume": _sanitize_number(row.get("volume")),
            "tte": tte_days,
            "underlying_price": underlying_price,
            "price_for_iv": option_price,
            "price_source": price_source,
            "iv": iv,
            "iv_reason": final_iv_reason,
            "delta": d,
            "gamma": g,
            "theta": th,
            "vega": v,
        })
    return {"schema_version": 1, "ts": ts, "rows": output_rows}, diagnostics


def build_curve_snapshot(market_rows, option_rows, focus_symbol=None) -> dict:
    ts = int(time.time() * 1000)
    if not market_rows:
        return {"schema_version": 1, "ts": ts, "rows": []}, {"focus_symbol": "", "focus_underlying": "", "contracts": 0}
    df = pd.DataFrame(market_rows)
    if df.empty:
        return {"schema_version": 1, "ts": ts, "rows": []}, {"focus_symbol": "", "focus_underlying": "", "contracts": 0}
    if "product_class" not in df.columns:
        return {"schema_version": 1, "ts": ts, "rows": []}, {"focus_symbol": "", "focus_underlying": "", "contracts": 0}
    class_tokens = df["product_class"].astype("string").str.strip()
    futures_df = df[class_tokens.fillna("") == "1"].copy()
    if futures_df.empty:
        return {"schema_version": 1, "ts": ts, "rows": []}, {"focus_symbol": "", "focus_underlying": "", "contracts": 0}

    options_df = pd.DataFrame(option_rows or [])
    if not options_df.empty:
        options_df["underlying"] = options_df.get("underlying", pd.Series(dtype="string")).astype("string")
        options_df["underlying_norm"] = options_df["underlying"].astype("string").str.strip().str.lower()
        options_df["option_type_cp"] = options_df.get("option_type", pd.Series(dtype="string")).astype("string").str.strip().str.lower()
        options_df["iv"] = pd.to_numeric(options_df.get("iv"), errors="coerce")
        options_df["delta"] = pd.to_numeric(options_df.get("delta"), errors="coerce")

    futures_df["ctp_contract"] = futures_df.get("ctp_contract", pd.Series(dtype="string")).astype("string")
    futures_df["contract_norm"] = futures_df["ctp_contract"].astype("string").str.strip().str.lower()
    futures_df["resolved_symbol"] = futures_df.apply(
        lambda row: _resolve_contract_symbol(row.get("ctp_contract"), row.get("symbol")),
        axis=1,
    )
    futures_df["resolved_symbol_norm"] = futures_df["resolved_symbol"].astype("string").str.strip().str.lower()
    futures_df["last"] = pd.to_numeric(futures_df.get("last"), errors="coerce")
    futures_df["volume"] = pd.to_numeric(futures_df.get("volume"), errors="coerce")
    futures_df["open_interest"] = pd.to_numeric(futures_df.get("open_interest"), errors="coerce")
    futures_df["bid1"] = pd.to_numeric(futures_df.get("bid1"), errors="coerce")
    futures_df["ask1"] = pd.to_numeric(futures_df.get("ask1"), errors="coerce")
    futures_df["bid_vol1"] = pd.to_numeric(futures_df.get("bid_vol1"), errors="coerce")
    futures_df["ask_vol1"] = pd.to_numeric(futures_df.get("ask_vol1"), errors="coerce")
    futures_df = futures_df.dropna(subset=["contract_norm", "last"])

    focus_norm = _norm_token(focus_symbol)
    focus_group = None
    if focus_norm is not None:
        focus_rows = futures_df[futures_df["contract_norm"] == focus_norm]
        if not focus_rows.empty:
            focus_row = focus_rows.iloc[0]
            focus_group = _norm_token(focus_row.get("resolved_symbol_norm"))
        if focus_group is None or focus_group == "":
            focus_group = _norm_token(_resolve_contract_symbol(focus_symbol, None))
        if focus_group is None or focus_group == "":
            # curve is a symbol-level view; root fallback is acceptable here as a
            # last resort group token when metadata-based symbol resolution fails
            focus_group = _contract_root(focus_symbol)
        if focus_group is not None and focus_group != "":
            futures_df = futures_df[futures_df["resolved_symbol_norm"] == focus_group]
        else:
            futures_df = futures_df[futures_df["contract_norm"] == focus_norm]

    futures_df = futures_df.sort_values(by="ctp_contract", kind="stable")

    rows_out = []
    for _, fut in futures_df.iterrows():
        contract = _sanitize_text(fut.get("ctp_contract"))
        forward = _sanitize_number(fut.get("last"))
        if contract is None or forward is None:
            continue
        vix_value = None
        call_skew = None
        put_skew = None
        if not options_df.empty:
            chain = options_df[options_df["underlying_norm"] == _norm_token(contract)]
            if not chain.empty:
                chain = chain[chain["iv"].notna() & chain["delta"].notna()]
                if not chain.empty:
                    vix_value = _mean_iv_in_abs_delta_band(chain, 0.25, 0.5)
                    call_skew = _compute_side_skew(chain[chain["option_type_cp"] == "c"])
                    put_skew = _compute_side_skew(chain[chain["option_type_cp"] == "p"])
        rows_out.append({
            "ctp_contract": contract,
            "forward": forward,
            "volume": _sanitize_number(fut.get("volume")),
            "open_interest": _sanitize_number(fut.get("open_interest")),
            "bid_vol1": _sanitize_number(fut.get("bid_vol1")),
            "bid1": _sanitize_number(fut.get("bid1")),
            "ask1": _sanitize_number(fut.get("ask1")),
            "ask_vol1": _sanitize_number(fut.get("ask_vol1")),
            "vix": vix_value,
            "call_skew": call_skew,
            "put_skew": put_skew,
        })
    return {"schema_version": 1, "ts": ts, "rows": rows_out}, {
        "focus_symbol": focus_norm or "",
        "focus_underlying": focus_group or "",
        "contracts": len(rows_out),
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="starSling options worker")
    parser.add_argument("--router_addr", required=True, help="Router tcp addr, e.g. 127.0.0.1:19090")
    parser.add_argument("--risk_free_rate", type=float, default=DEFAULT_RISK_FREE_RATE, help="Risk-free rate for option greeks")
    parser.add_argument("--days_in_year", type=int, default=int(PARAM_DAYS_IN_YEAR), help="Calendar days used for TTE year fraction")
    return parser.parse_args()


def main() -> int:
    global PARAM_DAYS_IN_YEAR
    args = parse_args()
    if args.days_in_year <= 0 or args.days_in_year > 370:
        log(f"invalid days_in_year: {args.days_in_year}")
        return 2
    PARAM_DAYS_IN_YEAR = float(args.days_in_year)
    router_target = parse_router_addr(args.router_addr)
    if router_target is None:
        log(f"invalid router_addr: {args.router_addr}")
        return 2
    if not HAS_VOLLIB:
        log("py_vollib unavailable; options worker will use fallback iv solver")
    last_seq = 0
    req_id = 1
    loop_count = 0
    backoff_seconds = POLL_INTERVAL_SECONDS
    cached_market_rows = []
    cached_option_rows = []
    last_curve_focus = None
    _safe_append_log(router_target, "INFO", "options_worker", "options worker started")
    if not HAS_VOLLIB:
        _safe_append_log(router_target, "WARN", "options_worker", "py_vollib unavailable; using fallback iv solver")
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
            ui_state = _rpc_call(router_target, "router.get_ui_state", {}, req_id)
            req_id += 1
            if not isinstance(ui_state, dict):
                ui_state = {}
            focus_symbol = _sanitize_text(ui_state.get("focus_symbol"))
            focus_norm = _norm_token(focus_symbol)
            if result.get("unchanged"):
                if focus_norm != last_curve_focus and cached_market_rows:
                    curve_snapshot, _ = build_curve_snapshot(
                        cached_market_rows,
                        cached_option_rows,
                        focus_symbol=focus_symbol,
                    )
                    _rpc_notify(router_target, "curve.snapshot", curve_snapshot)
                    last_curve_focus = focus_norm
                time.sleep(POLL_INTERVAL_SECONDS)
                continue
            seq = result.get("seq")
            next_seq = last_seq
            if isinstance(seq, (int, float)):
                next_seq = int(seq)
            rows = result.get("rows", [])
            if not isinstance(rows, list):
                rows = []
            options_snapshot, diagnostics = build_options_snapshot(rows, args.risk_free_rate)
            next_market_rows = rows
            next_option_rows = options_snapshot.get("rows", []) or []
            curve_snapshot, curve_diag = build_curve_snapshot(
                next_market_rows,
                next_option_rows,
                focus_symbol=focus_symbol,
            )
            _rpc_notify(router_target, "options.snapshot", options_snapshot)
            _rpc_notify(router_target, "curve.snapshot", curve_snapshot)
            cached_market_rows = next_market_rows
            cached_option_rows = next_option_rows
            last_curve_focus = focus_norm
            last_seq = next_seq
            loop_count += 1
            backoff_seconds = POLL_INTERVAL_SECONDS
            if loop_count == 1 or loop_count % HEALTH_LOG_INTERVAL == 0:
                failure_parts = []
                for key in sorted(diagnostics.get("iv_failures", {}).keys()):
                    failure_parts.append(f"{key}:{diagnostics['iv_failures'][key]}")
                failure_text = ",".join(failure_parts) if failure_parts else "-"
                _safe_append_log(
                    router_target,
                    "INFO",
                    "options_worker",
                    (
                        "options snapshot total={options_total} iv_valid={iv_valid} "
                        "price_valid={price_valid} vwap_valid={vwap_valid} "
                        "fallback_last={last_fallback_used} underlying_valid={underlying_valid} "
                        "iv_fail_total={iv_failure_total} iv_failures={iv_failures} "
                        "focus={focus} curve_under={curve_under} curve_contracts={curve_contracts} "
                        "has_vollib={has_vollib}"
                    ).format(
                        options_total=diagnostics.get("options_total", 0),
                        iv_valid=diagnostics.get("iv_valid", 0),
                        price_valid=diagnostics.get("price_valid", 0),
                        vwap_valid=diagnostics.get("vwap_valid", 0),
                        last_fallback_used=diagnostics.get("last_fallback_used", 0),
                        underlying_valid=diagnostics.get("underlying_valid", 0),
                        iv_failure_total=diagnostics.get("iv_failure_total", 0),
                        iv_failures=failure_text,
                        focus=curve_diag.get("focus_symbol", ""),
                        curve_under=curve_diag.get("focus_underlying", ""),
                        curve_contracts=curve_diag.get("contracts", 0),
                        has_vollib=1 if HAS_VOLLIB else 0,
                    ),
                )
                debug_rows = options_snapshot.get("rows", []) or []
                if focus_symbol is not None and focus_symbol != "":
                    focused_rows = [row for row in debug_rows if _option_row_matches_focus(row, focus_symbol)]
                    if focused_rows:
                        debug_rows = focused_rows
                for sample in debug_rows[:IV_DEBUG_SAMPLE_LIMIT]:
                    _safe_append_log(
                        router_target,
                        "DEBUG",
                        "options_worker",
                        (
                            "iv_debug contract={contract} cp_raw={cp_raw} cp={cp} "
                            "S={underlying} K={strike} px={price} tte={tte} iv={iv} reason={reason}"
                        ).format(
                            contract=sample.get("ctp_contract") or "-",
                            cp_raw=sample.get("option_type_raw") or "-",
                            cp=sample.get("option_type") or "-",
                            underlying=_fmt_debug_number(sample.get("underlying_price")),
                            strike=_fmt_debug_number(sample.get("strike")),
                            price=_fmt_debug_number(sample.get("price_for_iv")),
                            tte=_fmt_debug_number(sample.get("tte")),
                            iv=_fmt_debug_number(sample.get("iv")),
                            reason=sample.get("iv_reason") or "-",
                        ),
                    )
        except KeyboardInterrupt:
            log("options worker interrupted")
            return 0
        except Exception as exc:
            log(f"options worker loop error: {exc}")
            _safe_append_log(router_target, "ERROR", "options_worker", f"loop error: {exc}")
            time.sleep(backoff_seconds)
            backoff_seconds = min(backoff_seconds * 2, MAX_BACKOFF_SECONDS)


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Exhaustive consistency check for metadata resolve vs fallback inference.

Rules:
- All contract->symbol / option->underlying / option->cp conversions should use
  metadata resolve first.
- Fallback inference must converge to the same result when metadata mapping exists.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from dataclasses import dataclass
from typing import Dict, Iterable, List, Optional, Tuple


@dataclass
class ContractRow:
    contract: str
    raw_symbol: str
    underlying: str
    product_class: str
    option_cp: str


@dataclass
class ContractMapping:
    contract: str
    symbol: str
    raw_symbol: str
    underlying: str
    product_class: str
    option_cp: str


def _sanitize_text(value: object) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text if text else None


def _norm_token(value: object) -> Optional[str]:
    text = _sanitize_text(value)
    return text.lower() if text is not None else None


def _metadata_cache_dirs() -> List[str]:
    dirs: List[str] = []
    if sys.platform == "darwin":
        dirs.append(os.path.join(os.path.expanduser("~"), "Library", "Application Support", "starsling", "metadata"))
    else:
        xdg = os.getenv("XDG_CONFIG_HOME")
        if xdg:
            dirs.append(os.path.join(xdg, "starsling", "metadata"))
        else:
            dirs.append(os.path.join(os.path.expanduser("~"), ".config", "starsling", "metadata"))
    dirs.append(os.path.join(os.getcwd(), "runtime", "metadata"))
    return dirs


def _find_contract_metadata_file(explicit_path: Optional[str]) -> Optional[str]:
    candidates: List[str] = []
    if explicit_path:
        candidates.append(explicit_path)
    env_dir = _sanitize_text(os.getenv("STARSLING_METADATA_DIR"))
    if env_dir:
        candidates.append(os.path.join(env_dir, "contract.json"))
    for base in _metadata_cache_dirs():
        candidates.append(os.path.join(base, "contract.json"))
    for path in candidates:
        if os.path.isfile(path):
            return path
    return None


def _parse_contract_rows(payload: object) -> List[dict]:
    if payload is None:
        return []
    if isinstance(payload, list):
        return [row for row in payload if isinstance(row, dict)]
    if isinstance(payload, dict):
        data = payload.get("data")
        if isinstance(data, list):
            return [row for row in data if isinstance(row, dict)]
        if isinstance(data, dict):
            nested = data.get("data")
            if isinstance(nested, list):
                return [row for row in nested if isinstance(row, dict)]
    return []


def _load_rows(path: str) -> List[ContractRow]:
    with open(path, "r", encoding="utf-8") as handle:
        cached = json.load(handle)
    payload = cached.get("data") if isinstance(cached, dict) else None
    raw_rows = _parse_contract_rows(payload)
    out: List[ContractRow] = []
    for row in raw_rows:
        contract = _sanitize_text(row.get("InstrumentID"))
        if contract is None:
            continue
        out.append(
            ContractRow(
                contract=contract,
                raw_symbol=_sanitize_text(row.get("ProductID")) or "",
                underlying=_sanitize_text(row.get("UnderlyingInstrID")) or "",
                product_class=_sanitize_text(row.get("ProductClass")) or "",
                option_cp=_option_type_to_cp(row.get("OptionsType")),
            )
        )
    return out


def _option_type_to_cp(raw: object) -> str:
    token = (_sanitize_text(raw) or "").lower()
    if token in ("1", "c", "call", "认购"):
        return "c"
    if token in ("2", "p", "put", "认沽"):
        return "p"
    return ""


def _normalize_product_symbol(symbol: str) -> str:
    token = symbol.strip()
    if not token:
        return ""
    if "_" in token:
        token = token.split("_", 1)[0]
    return token.strip()


def _normalize_option_product_symbol(symbol: str, option_cp: str) -> str:
    base = _normalize_product_symbol(symbol)
    if len(base) <= 1:
        return base
    cp = option_cp.strip().lower()
    if cp == "c" and base[-1].lower() == "c":
        base = base[:-1]
    elif cp == "p" and base[-1].lower() == "p":
        base = base[:-1]
    return base.strip()


def _contract_root(contract: object) -> str:
    token = _sanitize_text(contract)
    if token is None:
        return ""
    idx = 0
    while idx < len(token) and token[idx].isalpha():
        idx += 1
    return token[:idx] if idx > 0 else ""


def _option_contract_cp_index(contract: object) -> int:
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
        suffix = upper[i + 1 :]
        if not suffix or not suffix.isdigit():
            continue
        prefix = upper[:i]
        if not any(c.isdigit() for c in prefix):
            continue
        return i
    return -1


_LEADING_CONTRACT_PATTERN = re.compile(r"^[A-Za-z]+\d+")


def _infer_option_type_cp(contract: object) -> str:
    token = _sanitize_text(contract)
    if token is None:
        return ""
    idx = _option_contract_cp_index(token)
    if idx < 0:
        return ""
    return "c" if token.upper()[idx] == "C" else "p"


def _infer_option_underlying_from_contract(contract: object) -> str:
    token = _sanitize_text(contract)
    if token is None:
        return ""
    idx = _option_contract_cp_index(token)
    if idx <= 0:
        return ""
    underlying = token[:idx].strip().rstrip("-_")
    matched = _LEADING_CONTRACT_PATTERN.match(underlying)
    if matched is not None:
        return matched.group(0)
    return underlying


def _replace_contract_root(contract: object, symbol: object) -> str:
    token = _sanitize_text(contract)
    symbol_token = _sanitize_text(symbol)
    if token is None or symbol_token is None:
        return ""
    root = _contract_root(token)
    if not root:
        return ""
    return symbol_token + token[len(root) :]


def _build_mappings(rows: Iterable[ContractRow]) -> Tuple[Dict[str, ContractMapping], Dict[str, str]]:
    by_contract: Dict[str, ContractMapping] = {}
    for row in rows:
        key = row.contract.lower()
        by_contract[key] = ContractMapping(
            contract=row.contract,
            symbol="",
            raw_symbol=row.raw_symbol,
            underlying=row.underlying,
            product_class=row.product_class,
            option_cp=row.option_cp,
        )

    for key, mapping in list(by_contract.items()):
        if mapping.product_class == "2":
            continue
        symbol = _normalize_product_symbol(mapping.raw_symbol)
        if not symbol:
            symbol = _contract_root(mapping.contract)
        mapping.symbol = symbol
        by_contract[key] = mapping

    option_root_to_under_symbol: Dict[str, str] = {}
    for key, mapping in list(by_contract.items()):
        if mapping.product_class != "2":
            continue
        symbol = ""
        under = mapping.underlying.strip()
        if under:
            under_mapping = by_contract.get(under.lower())
            if under_mapping is not None:
                symbol = _sanitize_text(under_mapping.symbol) or ""
        if not symbol:
            symbol = _normalize_option_product_symbol(mapping.raw_symbol, mapping.option_cp)
        if not symbol:
            symbol = _contract_root(mapping.contract)
        mapping.symbol = symbol
        by_contract[key] = mapping

        root = _contract_root(mapping.contract).lower()
        if root and symbol and root not in option_root_to_under_symbol:
            option_root_to_under_symbol[root] = symbol

    return by_contract, option_root_to_under_symbol


def _resolve_contract_symbol(by_contract: Dict[str, ContractMapping], contract: str) -> str:
    mapping = by_contract.get(contract.lower())
    if mapping is None:
        return ""
    return mapping.symbol.strip()


def _resolve_option_underlying(by_contract: Dict[str, ContractMapping], contract: str) -> str:
    mapping = by_contract.get(contract.lower())
    if mapping is None or mapping.product_class != "2":
        return ""
    return mapping.underlying.strip()


def _resolve_option_type_cp(by_contract: Dict[str, ContractMapping], contract: str) -> str:
    mapping = by_contract.get(contract.lower())
    if mapping is None:
        return ""
    return mapping.option_cp.strip().lower()


def _infer_contract_symbol(option_root_map: Dict[str, str], contract: str) -> str:
    root = _contract_root(contract)
    if not root:
        return ""
    mapped = option_root_map.get(root.lower())
    if mapped:
        return mapped
    return root


def _infer_option_underlying(option_root_map: Dict[str, str], contract: str) -> str:
    underlying = _infer_option_underlying_from_contract(contract)
    if not underlying:
        return ""
    root = _contract_root(contract)
    if not root:
        return underlying
    symbol = option_root_map.get(root.lower())
    if not symbol:
        return underlying
    replaced = _replace_contract_root(underlying, symbol)
    return replaced or underlying


def _build_option_variants(contract: str) -> List[str]:
    variants = {contract, contract.upper(), contract.lower()}
    idx = _option_contract_cp_index(contract)
    if idx > 0:
        cp = contract[idx]
        if cp.upper() in ("C", "P"):
            prefix = contract[:idx].rstrip("-_")
            suffix = contract[idx + 1 :].lstrip("-_")
            if prefix and suffix:
                compact = f"{prefix}{cp}{suffix}"
                hyphenated = f"{prefix}-{cp}-{suffix}"
                variants.update({compact, compact.upper(), compact.lower(), hyphenated, hyphenated.upper(), hyphenated.lower()})
    return sorted(variants)


def _same(a: str, b: str) -> bool:
    return _norm_token(a) == _norm_token(b)


def main() -> int:
    parser = argparse.ArgumentParser(description="Check metadata mapping and fallback inference consistency.")
    parser.add_argument("--path", help="Path to contract.json cache file", default=None)
    args = parser.parse_args()

    path = _find_contract_metadata_file(args.path)
    if path is None:
        print("ERROR: contract metadata file not found.")
        return 2

    rows = _load_rows(path)
    if not rows:
        print(f"ERROR: no contract rows parsed from {path}")
        return 2

    by_contract, option_root_map = _build_mappings(rows)

    symbol_mismatches: List[str] = []
    underlying_mismatches: List[str] = []
    cp_mismatches: List[str] = []
    variant_mismatches: List[str] = []

    option_rows = [row for row in by_contract.values() if row.product_class == "2"]
    for row in by_contract.values():
        meta_symbol = _resolve_contract_symbol(by_contract, row.contract)
        infer_symbol = _infer_contract_symbol(option_root_map, row.contract)
        if not _same(meta_symbol, infer_symbol):
            symbol_mismatches.append(
                f"{row.contract}: meta_symbol={meta_symbol!r}, infer_symbol={infer_symbol!r}, raw_symbol={row.raw_symbol!r}"
            )

        if row.product_class != "2":
            continue

        meta_under = _resolve_option_underlying(by_contract, row.contract)
        infer_under = _infer_option_underlying(option_root_map, row.contract)
        if not _same(meta_under, infer_under):
            underlying_mismatches.append(
                f"{row.contract}: meta_underlying={meta_under!r}, infer_underlying={infer_under!r}, symbol={row.symbol!r}"
            )

        meta_cp = _resolve_option_type_cp(by_contract, row.contract)
        infer_cp = _infer_option_type_cp(row.contract)
        if not _same(meta_cp, infer_cp):
            cp_mismatches.append(f"{row.contract}: meta_cp={meta_cp!r}, infer_cp={infer_cp!r}")

        expected_symbol = infer_symbol
        expected_under = infer_under
        expected_cp = infer_cp
        for variant in _build_option_variants(row.contract):
            got_symbol = _infer_contract_symbol(option_root_map, variant)
            got_under = _infer_option_underlying(option_root_map, variant)
            got_cp = _infer_option_type_cp(variant)
            if not (_same(expected_symbol, got_symbol) and _same(expected_under, got_under) and _same(expected_cp, got_cp)):
                variant_mismatches.append(
                    "{} -> variant={} expected(symbol={}, underlying={}, cp={}) got(symbol={}, underlying={}, cp={})".format(
                        row.contract,
                        variant,
                        expected_symbol,
                        expected_under,
                        expected_cp,
                        got_symbol,
                        got_under,
                        got_cp,
                    )
                )

    print(f"metadata_file={path}")
    print(f"contracts_total={len(by_contract)}")
    print(f"options_total={len(option_rows)}")
    print(f"option_root_mappings={len(option_root_map)}")
    print(f"symbol_mismatches={len(symbol_mismatches)}")
    print(f"underlying_mismatches={len(underlying_mismatches)}")
    print(f"cp_mismatches={len(cp_mismatches)}")
    print(f"variant_mismatches={len(variant_mismatches)}")

    irregular = 0
    for row in option_rows:
        if "_" in row.raw_symbol:
            irregular += 1
    print(f"irregular_option_rows_with_underscore={irregular}")

    def _emit(name: str, items: List[str]) -> None:
        if not items:
            return
        print(f"\n{name} (showing up to 20):")
        for line in items[:20]:
            print(f"- {line}")

    _emit("symbol_mismatches", symbol_mismatches)
    _emit("underlying_mismatches", underlying_mismatches)
    _emit("cp_mismatches", cp_mismatches)
    _emit("variant_mismatches", variant_mismatches)

    if symbol_mismatches or underlying_mismatches or cp_mismatches or variant_mismatches:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

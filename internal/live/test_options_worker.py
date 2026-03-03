import math

import pandas as pd

from internal.live import options_worker


def test_compute_side_skew_prefers_primary_iv25_band():
    chain = pd.DataFrame(
        [
            {"delta": 0.50, "iv": 0.20},
            {"delta": 0.25, "iv": 0.30},
            {"delta": 0.35, "iv": 0.01},
        ]
    )

    skew = options_worker._compute_side_skew(chain)

    assert math.isclose(skew, 0.10, rel_tol=0.0, abs_tol=1e-12)


def test_compute_side_skew_uses_fallback_iv25_band_when_primary_missing():
    chain = pd.DataFrame(
        [
            {"delta": 0.50, "iv": 0.20},
            {"delta": 0.35, "iv": 0.30},
        ]
    )

    skew = options_worker._compute_side_skew(chain)

    assert math.isclose(skew, 0.10, rel_tol=0.0, abs_tol=1e-12)


def test_compute_side_skew_uses_second_fallback_when_first_fallback_missing():
    chain = pd.DataFrame(
        [
            {"delta": 0.50, "iv": 0.20},
            {"delta": 0.05, "iv": 0.35},
        ]
    )

    skew = options_worker._compute_side_skew(chain)

    assert math.isclose(skew, 0.075, rel_tol=0.0, abs_tol=1e-12)


def test_compute_side_skew_returns_none_without_atm_samples():
    chain = pd.DataFrame(
        [
            {"delta": 0.25, "iv": 0.30},
            {"delta": 0.35, "iv": 0.20},
        ]
    )

    assert options_worker._compute_side_skew(chain) is None


def test_build_curve_snapshot_fallback_is_side_local():
    market_rows = [
        {
            "product_class": "1",
            "ctp_contract": "cu2604",
            "symbol": "cu",
            "last": 100.0,
            "volume": 10,
            "open_interest": 20,
        }
    ]
    option_rows = [
        {"underlying": "cu2604", "option_type": "c", "iv": 0.30, "delta": 0.35},
        {"underlying": "cu2604", "option_type": "c", "iv": 0.20, "delta": 0.50},
        {"underlying": "cu2604", "option_type": "p", "iv": 0.27, "delta": -0.25},
        {"underlying": "cu2604", "option_type": "p", "iv": 0.19, "delta": -0.50},
        {"underlying": "cu2604", "option_type": "p", "iv": 0.01, "delta": -0.35},
    ]

    snapshot, _ = options_worker.build_curve_snapshot(market_rows, option_rows)
    rows = snapshot.get("rows", [])

    assert len(rows) == 1
    row = rows[0]
    assert math.isclose(row["call_skew"], 0.10, rel_tol=0.0, abs_tol=1e-12)
    assert math.isclose(row["put_skew"], 0.08, rel_tol=0.0, abs_tol=1e-12)

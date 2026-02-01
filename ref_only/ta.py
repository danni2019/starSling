import pandas as pd
import numpy as np


def spot_highs_with_no_future_looks(s: pd.Series, N: int):
    _compare = s > s.shift(1).rolling(N).max()
    _compare = (_compare.shift(1).fillna(False)) & (~_compare)
    _compare = _compare.replace(False, np.nan)
    idx = _compare.dropna().index
    return s.loc[idx]


def spot_lows_with_no_future_looks(s: pd.Series, N: int):
    _compare = s < s.shift(1).rolling(N).min()
    _compare = (_compare.shift(1).fillna(False)) & (~_compare)
    _compare = _compare.replace(False, np.nan)
    idx = _compare.dropna().index
    return s.loc[idx]


def spot_highs_with_future_looks(s: pd.Series, N: int):
    _compare = s > s.shift(1).rolling(N).max()
    _compare = _compare & (~_compare.shift(-1).fillna(False))
    _compare = _compare.replace(False, np.nan)
    idx = _compare.dropna().index
    return s.loc[idx]


def spot_lows_with_future_looks(s: pd.Series, N: int):
    _compare = s < s.shift(1).rolling(N).min()
    _compare = _compare & (~_compare.shift(-1).fillna(False))
    _compare = _compare.replace(False, np.nan)
    idx = _compare.dropna().index
    return s.loc[idx]


def dow_theory_indicator(df: pd.DataFrame):
    if "highs" not in df.columns or "lows" not in df.columns:
        raise ValueError("Missing columns: must contain highs and lows columns.")
    sample_df = df.copy()
    sample_df['highs'] = sample_df['highs'].ffill()
    sample_df['lows'] = sample_df['lows'].ffill()
    sample_df['hh'] = (sample_df['highs'] > sample_df['highs'].shift(1)).astype(int)
    sample_df['hl'] = (sample_df['lows'] > sample_df['lows'].shift(1)).astype(int)
    sample_df['ll'] = (sample_df['lows'] < sample_df['lows'].shift(1)).astype(int) * (-1)
    sample_df['lh'] = (sample_df['highs'] < sample_df['highs'].shift(1)).astype(int) * (-1)
    sample_df['high_indicator'] = (sample_df['hh'] + sample_df['lh']).replace(0, np.nan).ffill()
    sample_df['low_indicator'] = (sample_df['hl'] + sample_df['ll']).replace(0, np.nan).ffill()
    sample_df['indicator'] = np.where(
        sample_df['high_indicator'] * sample_df['low_indicator'] > 0,
        sample_df['high_indicator'],
        0
    )
    return sample_df['indicator']


def performance_metrics(returns, freq=252, rf=0.0):
    """returns: pd.Series of periodic returns (e.g. daily)
       freq: periods per year (252=daily, 12=monthly)
       rf: risk-free rate (annualized)
    """
    # drop NaN
    r = returns.dropna()
    if len(r) == 0:
        return {}

    # annualized return (CAGR)
    total_return = (1 + r).prod() - 1
    annual_return = (1 + total_return)**(freq / len(r)) - 1

    # annualized volatility
    annual_vol = r.std() * np.sqrt(freq)

    # Sharpe ratio
    excess_daily = r - rf / freq
    sharpe = np.mean(excess_daily) / np.std(excess_daily, ddof=1) * np.sqrt(freq)

    # max drawdown
    cum = (1 + r).cumprod()
    peak = cum.cummax()
    drawdown = (cum / peak - 1)
    max_dd = drawdown.min()

    # Calmar ratio
    calmar = annual_return / abs(max_dd) if max_dd != 0 else np.nan

    # Sortino ratio (downside volatility only)
    downside = r[r < 0]
    downside_std = downside.std() * np.sqrt(freq)
    sortino = (np.mean(r) * freq) / downside_std if downside_std != 0 else np.nan

    # Win rate
    win_rate = (r > 0).sum() / len(r)

    # hit ratio of positive returns
    avg_gain = r[r > 0].mean()
    avg_loss = r[r < 0].mean()

    # skewness & kurtosis
    skew = r.skew()
    kurt = r.kurtosis()

    return {
        'Annual Return': annual_return,
        'Annual Volatility': annual_vol,
        'Sharpe Ratio': sharpe,
        'Max Drawdown': max_dd,
        'Calmar Ratio': calmar,
        'Sortino Ratio': sortino,
        'Win Rate': win_rate,
        'Avg Gain': avg_gain,
        'Avg Loss': avg_loss,
        'Skew': skew,
        'Kurtosis': kurt,
    }


def realized_vol(p: pd.Series, n: int):
    # 使用给定收益率 series 计算历史波动率（年化），窗口 n
    # 约定：p 为周期收益率（如日度对数收益），输出与 p 对齐
    if p is None:
        return pd.Series(dtype=float)
    s = pd.to_numeric(p, errors='coerce')
    # 使用 min_periods=n 保证窗口未满前返回 NaN；年化按日频 sqrt(255)
    rv = s.rolling(window=n, min_periods=n).std() * np.sqrt(255)
    return rv

import pandas as pd
import numpy as np
from option_calc import OptionModel as om

RISK_FREE_RATE = 0.01
DAYS_IN_YEAR = 365


class OptionGreeks:
    def __init__(self, live_md_handle):
        self.live_handle = live_md_handle

    def get_live_md(self):
        return self.live_handle.get_live_md()

    def calculate_option_greeks(self):
        md_df = self.get_live_md()
        if not isinstance(md_df, pd.DataFrame) or md_df.empty:
            return
        
        md = md_df.copy()
        # step1: 计算vwap，并用last补充异常值
        den = md[['ask_vol1','bid_vol1']].sum(axis=1).replace(0, np.nan)
        md['price'] = (md['ask1'] * md['ask_vol1'] + md['bid1'] * md['bid_vol1']) / den
        md['price'] = md['price'].fillna(md['last'])
        #step2: 将underlying price填充至 option_df，并仅保留具备underlying price的记录
        future_df = md[~md['contract'].str.contains('-')].copy()
        future_df = future_df.sort_values(by='datetime', ascending=False).drop_duplicates(subset=['contract'], keep='first').set_index('contract', drop=True)
        future_price_map = future_df['price'].to_dict()

        option_df = md[md['contract'].str.contains('-')].copy()
        if option_df.empty:
            return
        option_df[['underlying_contract', 'direction', 'strike', 'option_type']] = option_df['contract'].str.split('-', expand=True)
        option_df['underlying_price'] = option_df['underlying_contract'].map(
            lambda x: future_price_map.get(x, np.nan)
        )

        option_df = option_df.dropna(subset=['underlying_price'])
        if option_df.empty:
            return
        
        # step3: 处理日期数据类型
        option_df['delist_date'] = pd.to_datetime(option_df['delist_date'], format="%Y-%m-%d %H:%M:%S", errors='coerce')
        option_df['trading_date'] = pd.to_datetime(option_df['trading_date'], format="%Y-%m-%d %H:%M:%S", errors='coerce')
        option_df['datetime'] = pd.to_datetime(option_df['datetime'], format="%Y-%m-%d %H:%M:%S", errors='coerce')
        option_df['datetime_minute'] = pd.to_datetime(option_df['datetime_minute'], format="%Y-%m-%d %H:%M:%S", errors='coerce')

        # step4: 进行必要的初级计算
        option_df['direction_factor'] = np.where(option_df['direction'] == 'C', 1, -1)
        option_df['otm_pctg'] = option_df['direction_factor'] * (option_df['strike'].astype(float) / option_df['underlying_price'] - 1)
        option_df['otm_dist'] = option_df['otm_pctg'].abs()

        # step5：处理atm strike & price

        atm_strike_map = (
            option_df.sort_values('otm_dist', ascending=True)
                    .groupby('underlying_contract')['strike']
                    .first()
        )

        option_df = option_df.join(
            atm_strike_map.rename('atm_strike'),
            on='underlying_contract'
        )

        def get_atm_price(row, df):
            # 从同一 underlying_contract 下找到 strike==atm_strike 且方向相同的一条
            subset = df[
                (df['underlying_contract'] == row['underlying_contract']) &
                (df['strike'] == row['atm_strike'])
            ]
            # 如果没有这个 strike（深度OTM/缺档），返回 NaN
            return subset['price'].iloc[0] if not subset.empty else np.nan

        call_df = option_df[option_df['direction'] == 'C']
        put_df  = option_df[option_df['direction'] == 'P']

        atm_call_price = option_df.apply(lambda r: get_atm_price(r, call_df), axis=1)
        atm_put_price = option_df.apply(lambda r: get_atm_price(r, put_df), axis=1)
        option_df['atm_price'] = np.where(
            option_df['direction'] == 'C',
            atm_call_price,
            atm_put_price
        )

        # step6: 仅看OTM期权
        option_df = option_df[option_df['otm_pctg'] > 0].copy()     # 仅看虚值期权

        # step7: 处理波动率
        expiry_days = (option_df['delist_date'] - option_df['trading_date']).dt.days
        option_df['expiry'] = np.where(expiry_days == 0, 0.1, expiry_days)
        option_df = option_df.dropna(subset=['expiry'])

        option_df['iv'] = [om.future_option_imp_vol(s['direction'].lower(), float(s['underlying_price']), float(s['strike']), float(s['expiry']), RISK_FREE_RATE, float(s['price'])) for _, s in option_df.iterrows()]
        option_df['rv'] = [self.rv.get(i, np.nan) for i in option_df['underlying_contract']]
        option_df['rv'] = option_df['rv'].fillna(option_df['iv'])

        # step8: 计算理论价与premium
        option_df['theo_price'] = [om.future_option_theo_price(s['direction'].lower(), float(s['underlying_price']), float(s['strike']), float(s['expiry']), RISK_FREE_RATE, float(s['rv'])) for _, s in option_df.iterrows()]
        option_df['premium'] = option_df['price'] / option_df['theo_price'].replace(0, np.nan) - 1

        # step9: 计算目标价与止损价，并评估赔率
        option_df['avail_days'] = np.where(option_df['expiry'] > OPTION_OBSERVE_DAYS, OPTION_OBSERVE_DAYS, option_df['expiry'])

        option_df['possible_range'] = (
            IV_MOVE_Z
            * option_df['iv']
            * np.sqrt(option_df['avail_days'] / DAYS_IN_YEAR)
        )

        option_df['target_bound'] = (
            option_df['underlying_price']
            * np.exp(option_df['direction_factor'] * option_df['possible_range'])
        )

        option_df['stop_bound'] = (
            option_df['underlying_price']
            * np.exp(-option_df['direction_factor'] * option_df['possible_range'])
        )
        option_df['ref_stop_bound'] = (
            option_df['underlying_price']
            * (1 - option_df['otm_pctg'] * option_df['direction_factor'])
        )

        def _price_option_with_bound(row, bound_key: str) -> float:
            tte = float(row['avail_days'])
            underlying = float(row[bound_key])
            strike = float(row['strike'])
            direction = str(row['direction']).lower()
            if tte <= 0:
                intrinsic = row['direction_factor'] * (underlying - strike)
                return float(max(intrinsic, 0.0))
            return om.future_option_theo_price(
                direction,
                underlying,
                strike,
                tte,
                RISK_FREE_RATE,
                float(row['iv']),
            )

        option_df['target_price_raw'] = [
            _price_option_with_bound(s, 'target_bound') for _, s in option_df.iterrows()
        ]
        option_df['stop_price_raw'] = [
            _price_option_with_bound(s, 'stop_bound') for _, s in option_df.iterrows()
        ]
        option_df['ref_stop_price_raw'] = [
            _price_option_with_bound(s, 'ref_stop_bound') for _, s in option_df.iterrows()
        ]

        option_df['theta_decay'] = [
            om.future_option_theta(
                s['direction'].lower(),
                float(s['underlying_price']),
                float(s['strike']),
                float(s['expiry']),
                RISK_FREE_RATE,
                float(s['iv']),
            )
            for _, s in option_df.iterrows()
        ]
        option_df['daily_theta'] = option_df['theta_decay'] / DAYS_IN_YEAR
        option_df['theta_loss'] = option_df['daily_theta'] * option_df['avail_days']

        # Blend IV-based stop with strike-distance stop to keep downside adaptive but bounded
        stop_blend = pd.concat(
            [option_df['stop_price_raw'], option_df['ref_stop_price_raw']],
            axis=1
        ).mean(axis=1, skipna=True)
        option_df['stop_price'] = (
            stop_blend + option_df['theta_loss']
        ).clip(lower=0.0)

        # Cap upside at ATM option price when available, otherwise keep IV target
        target_cap = pd.concat(
            [option_df['target_price_raw'], option_df['atm_price']],
            axis=1
        ).min(axis=1, skipna=True)
        option_df['target_price'] = (
            target_cap + option_df['theta_loss']
        ).clip(lower=0.0)

        denom = option_df['price'] - option_df['stop_price']
        denom = denom.replace(0, np.nan)
        option_df['payoff_ratio'] = (
            option_df['target_price'] - option_df['price']
        ) / denom
        option_df = option_df[option_df['payoff_ratio'] > 0].copy()

        # step11: 计算gamma delta库存
        option_df['gamma'] = [om.future_option_gamma(s['direction'].lower(), float(s['underlying_price']), float(s['strike']), float(s['expiry']), RISK_FREE_RATE, float(s['iv'])) for _, s in option_df.iterrows()]
        option_df['delta'] = [om.future_option_delta(s['direction'].lower(), float(s['underlying_price']), float(s['strike']), float(s['expiry']), RISK_FREE_RATE, float(s['iv'])) for _, s in option_df.iterrows()]
        option_df['gamma_inventory'] = option_df['gamma'] * option_df['open_interest']
        option_df['delta_inventory'] = option_df['delta'] * option_df['open_interest']

        # step12: 计算underlying对应的VIX近似
        iv_sample = option_df[(option_df['delta'].abs() <= 0.5) & (option_df['delta'].abs() >= 0.25)].copy()
        agg_cols = [col for col in ['iv', 'expiry'] if col in iv_sample.columns]
        if agg_cols:
            vix_df = iv_sample.groupby(['underlying_contract', 'symbol', 'exchange'])[agg_cols].mean()
            vix_df = vix_df.reset_index(drop=False)
        else:
            vix_df = iv_sample[['underlying_contract', 'symbol', 'exchange']].drop_duplicates().reset_index(drop=True)

        res_opt = option_df[[
            'trading_date', 'datetime', 'exchange', 'symbol', 'underlying_contract', 'contract', 'direction', 'underlying_price', 'price', 
            'iv', 'rv', 'premium', 'target_bound', 'stop_bound', 'payoff_ratio', 'target_price', 'stop_price', 'expiry', 'otm_dist'
        ]].copy()

        res_inventory = option_df[[
            'trading_date', 'datetime', 'exchange', 'symbol', 'underlying_contract', 'contract', 'direction', 'underlying_price', 'price', 
            'iv', 'gamma_inventory', 'delta_inventory', 'strike', 'option_type'
        ]].copy()
        
        return res_opt, res_inventory, vix_df
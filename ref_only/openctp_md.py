import pandas as pd
import numpy as np
from time import sleep
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo
from typing import Callable, Optional
from lib.orm.storage import DataStore
from lib.tool import standard_contract_to_ctp_code

from openctp_ctp import thostmduserapi as mdapi 



class CMdImpl(mdapi.CThostFtdcMdSpi):
    def __init__(self, md_front):
        mdapi.CThostFtdcMdSpi.__init__(self)
        self.md_front = md_front

        self.api = None

        self.latest_md_buf = {}
        self.contract_map_df = None

        self.db = DataStore()

        self.__tz_dt_format__ = "%Y-%m-%dT00:00:00+08:00"
        self.__tz__ = ZoneInfo("Asia/Shanghai")    # TZ info 仅用于从数据库中取合约数据。实时行情中的所有日期时间相关数据均不携带时区信息！

        self.md_status = False

        # optional supervisor notification when a new market tick arrives
        self._on_md_update: Optional[Callable[[], None]] = None

        current_t = self.get_current_trading_date()
        self.now = current_t

    def get_current_trading_date(self):
        now_ = datetime.now(self.__tz__)
        now_date = now_ if now_.hour < 18 else now_ + timedelta(days=1)
        now_date = now_date.replace(hour=0, minute=0, second=0, microsecond=0)
        trading_date = self.db.read_dataframe(
            "ref",
            "processed_cn_trading_calendar",
            filter_keyword={'trading_date': {'gte': now_date.strftime('%Y-%m-%d')}},
            ascending=[('trading_date', True)],
            limit=1
        )
        if trading_date.empty:
            print("Unable to get trading_date data from database! Using current date as fallback! "
                "Be aware of possible pitfalls!")
            return now_date
        current_trading_date = trading_date.iloc[0]['trading_date']
        return current_trading_date
    
    def future_contract_targets(self):
        contract_df = self.db.read_dataframe(
            "ref",
            "processed_cn_future_contract",
            filter_keyword={'delist_date': {'gte': self.now.strftime(self.__tz_dt_format__)}}
        )
        if contract_df.empty:
            return pd.DataFrame()
        contract_df['ctp_contract'] = [standard_contract_to_ctp_code(r['contract'], r['exchange']) for _, r in contract_df.iterrows()]
        # return contract_df['ctp_contract'].drop_duplicates().tolist()
        return contract_df[['contract', 'exchange', 'symbol', 'ctp_contract', 'delist_date']].copy()
    
    def option_contract_targets(self):
        contract_df = self.db.read_dataframe(
            "ref",
            "processed_cn_option_contract",
            filter_keyword={'delist_date': {'gte': self.now.strftime(self.__tz_dt_format__)}}
        )
        if contract_df.empty:
            return pd.DataFrame()
        # ctp 目前商品期货执行价均无小数，此处简单处理。
        contract_df['strike'] = contract_df['strike'].astype(int).astype(str)
        contract_df['ctp_contract'] = [standard_contract_to_ctp_code(r['underlying_contract'], r['exchange'], r['strike'], r['option_type']) for _, r in contract_df.iterrows()]
        # return contract_df['ctp_contract'].drop_duplicates().tolist()
        return contract_df[['underlying_contract', 'exchange', 'strike', 'option_type', 'direction', 'symbol', 'ctp_contract', 'delist_date']].copy()
    
    def all_subscription_contracts(self):
        fut_t = self.future_contract_targets()
        opt_t = self.option_contract_targets()
        if fut_t.empty:
            fut_ls = [] 
            fut_contract_map = fut_t
        else:
            fut_ls = fut_t['ctp_contract'].drop_duplicates().tolist()
            fut_contract_map = fut_t.set_index('ctp_contract', drop=True)
        
        if opt_t.empty:
            opt_ls = [] 
            opt_contract_map = opt_t
        else:
            opt_ls = opt_t['ctp_contract'].drop_duplicates().tolist()
            opt_contract_map = opt_t.set_index('ctp_contract', drop=True)
            opt_contract_map['contract'] = opt_contract_map['underlying_contract'] + '-' + opt_contract_map['direction'] + '-' + opt_contract_map['strike'] + '-' + opt_contract_map['option_type']
            opt_contract_map = opt_contract_map[['contract', 'exchange', 'symbol', 'delist_date']].copy()

        contract_map = pd.concat([fut_contract_map, opt_contract_map], axis=0)

        if not contract_map.empty:
            contract_map['delist_date'] = contract_map['delist_date'].dt.tz_localize(None)

        t = [
            *fut_ls,
            *opt_ls 
        ]
        return t, contract_map

    def Run(self):
        self.api = mdapi.CThostFtdcMdApi.CreateFtdcMdApi()
        self.api.RegisterFront(self.md_front)
        self.api.RegisterSpi(self)
        self.api.Init()

    def set_on_md(self, cb: Callable[[], None]) -> None:
        """Register a lightweight callback for market data updates.

        The callback should be thread-safe. Typically, the supervisor passes a
        closure that uses loop.call_soon_threadsafe to notify an asyncio.Event.
        """
        self._on_md_update = cb

    def OnFrontConnected(self) -> "void":
        # Market channel doesn't check userid and password.
        req = mdapi.CThostFtdcReqUserLoginField()
        self.api.ReqUserLogin(req, 0)

    def OnFrontDisconnected(self, nReason: int) -> "void":
        print(f"OnFrontDisconnected.[nReason={nReason}] | Status set to False")
        self.md_status = False

    def OnRspUserLogin(self, pRspUserLogin: 'CThostFtdcRspUserLoginField', pRspInfo: 'CThostFtdcRspInfoField', nRequestID: 'int', bIsLast: 'bool') -> "void":
        if pRspInfo is not None and pRspInfo.ErrorID != 0:
            print(f"Login failed. {pRspInfo.ErrorMsg}")
            return
        print(f"Login succeed.{pRspUserLogin.TradingDay}")

        c_list, self.contract_map_df = self.all_subscription_contracts()
        self.api.SubscribeMarketData([c.encode("utf-8") for c in c_list], len(c_list))

        self.md_status = True
        print(f"Subscribed {len(c_list)} contracts. Status set to True")

    def OnRtnDepthMarketData(self, pDepthMarketData: 'CThostFtdcDepthMarketDataField') -> "void":
        md_data = {
            "TradingDay": pDepthMarketData.TradingDay,
            "InstrumentID": pDepthMarketData.InstrumentID,
            "ExchangeID": pDepthMarketData.ExchangeID,
            "ExchangeInstID": pDepthMarketData.ExchangeInstID,
            "LastPrice": pDepthMarketData.LastPrice,
            "PreSettlementPrice": pDepthMarketData.PreSettlementPrice,
            "PreClosePrice": pDepthMarketData.PreClosePrice,
            "PreOpenInterest": pDepthMarketData.PreOpenInterest,
            "OpenPrice": pDepthMarketData.OpenPrice,
            "HighestPrice": pDepthMarketData.HighestPrice,
            "LowestPrice": pDepthMarketData.LowestPrice,
            "Volume": pDepthMarketData.Volume,
            "Turnover": pDepthMarketData.Turnover,
            "OpenInterest": pDepthMarketData.OpenInterest,
            "ClosePrice": pDepthMarketData.ClosePrice,
            "SettlementPrice": pDepthMarketData.SettlementPrice,
            "UpperLimitPrice": pDepthMarketData.UpperLimitPrice,
            "LowerLimitPrice": pDepthMarketData.LowerLimitPrice,
            "PreDelta": pDepthMarketData.PreDelta,
            "CurrDelta": pDepthMarketData.CurrDelta,
            "UpdateTime": pDepthMarketData.UpdateTime,
            "UpdateMillisec": pDepthMarketData.UpdateMillisec,
            "BidPrice1": pDepthMarketData.BidPrice1,
            "BidVolume1": pDepthMarketData.BidVolume1,
            "AskPrice1": pDepthMarketData.AskPrice1,
            "AskVolume1": pDepthMarketData.AskVolume1,
            "AveragePrice": pDepthMarketData.AveragePrice,
            "ActionDay": pDepthMarketData.ActionDay
        }
        self.latest_md_buf[pDepthMarketData.InstrumentID] = md_data
        try:
            if self._on_md_update is not None:
                self._on_md_update()
        except Exception:
            # never let callback exceptions bubble into CTP thread
            pass

    def OnRspSubMarketData(self, pSpecificInstrument: 'CThostFtdcSpecificInstrumentField', pRspInfo: 'CThostFtdcRspInfoField', nRequestID: 'int', bIsLast: 'bool') -> "void":
        pass

    def proc_raw_md_data(self, reserve_df: pd.DataFrame, trading_dt, act_dt):
        if self.contract_map_df is None or self.contract_map_df.empty:
            raise ValueError(f"No valid contract map dataframe available. Map is {self.contract_map_df}")
        contract_df = self.contract_map_df.copy()
        reserve_df[['contract', 'exchange', 'symbol', 'delist_date']] = contract_df.loc[
            contract_df.index.intersection(reserve_df.index)
        ]
        cond = reserve_df[
                   reserve_df.dtypes[
                       reserve_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()
               ] > 1e10
        reserve_df[
            reserve_df.dtypes[reserve_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()
        ] = reserve_df[
            reserve_df.dtypes[reserve_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()
        ].mask(cond, np.nan)
        reserve_df['TradingDay'] = pd.to_datetime(trading_dt)
        reserve_df['ActionDay'] = pd.to_datetime(act_dt)
        reserve_df['datetime'] = pd.to_datetime(
            reserve_df['ActionDay'].astype(str) +
            " " +
            reserve_df['UpdateTime'] +
            "." +
            reserve_df['UpdateMillisec'].astype(str)
        )
        reserve_df['datetime_minute'] = pd.to_datetime(
            reserve_df['ActionDay'].astype(str) +
            " " +
            reserve_df['UpdateTime']
        ).dt.ceil("1min")
        reserve_df = reserve_df[
            (reserve_df['datetime'] <= (datetime.now() + timedelta(minutes=2))) &
            (reserve_df['datetime'] >= (datetime.now() + timedelta(minutes=-2)))
            ].copy()

        return reserve_df

    @staticmethod
    def pretreat_md(raw_df: pd.DataFrame):
        return raw_df.drop(
            columns=[
                'ExchangeID', 'ExchangeInstID', 'PreDelta', 'CurrDelta',
                'UpdateTime', 'UpdateMillisec', 'ActionDay'
            ]
        ).rename(
            columns={
                'TradingDay': 'trading_date',
                'InstrumentID': 'ctp_contract',
                'LastPrice': 'last',
                'PreSettlementPrice': 'pre_settlement',
                'PreClosePrice': 'pre_close',
                'PreOpenInterest': 'pre_open_interest',
                'OpenPrice': 'open',
                'HighestPrice': 'high',
                'LowestPrice': 'low',
                'Volume': 'volume',
                'Turnover': 'turnover',
                'OpenInterest': 'open_interest',
                'ClosePrice': 'close',
                'SettlementPrice': 'settlement',
                'UpperLimitPrice': 'limit_up',
                'LowerLimitPrice': 'limit_down',
                'BidPrice1': 'bid1',
                'AskPrice1': 'ask1',
                'BidVolume1': 'bid_vol1',
                'AskVolume1': 'ask_vol1',
                'AveragePrice': 'average_price',

            }
        ).set_index('ctp_contract', drop=False)
    
    def get_current_md_df(self):
        raw_md = self.latest_md_buf.copy()
        real_trading_date = self.now.replace(tzinfo=None)
        real_action_date = datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)
        reserve_df = pd.DataFrame.from_dict(raw_md, orient='index')
        if reserve_df.empty:
            return
        else:
            try:
                raw_df = self.proc_raw_md_data(reserve_df, real_trading_date, real_action_date)
                pretreated_df = self.pretreat_md(raw_df)
            except Exception as e:
                print(f"Error: {e.args[0]}")
                return 
            else:
                return pretreated_df


if __name__ == '__main__':
    md = CMdImpl("tcp://180.166.37.178:41215")
    print(md.now)

    md.Run()

    input("Press enter key to exit.")

import argparse
import json
import os
import signal
import sys
import time
from datetime import datetime, timedelta
import pandas as pd
import numpy as np

from openctp_ctp import thostmduserapi as mdapi

LOGIN_TIMEOUT_SECONDS = 60

def log(message: str) -> None:
    print(message, file=sys.stderr, flush=True)


def parse_instruments(raw: str) -> list[str]:
    if not raw:
        return []
    return [item.strip() for item in raw.split(",") if item.strip()]


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


class MdSpi(mdapi.CThostFtdcMdSpi):
    def __init__(self, front: str, instruments: list[str], username: str, password: str) -> None:
        super().__init__()
        self.front = front
        self.instruments = instruments
        self.username = username
        self.password = password
        self.api = None
        self.running = True
        self.login_ok = False
        self.login_failed = False
        self.failure_reason = ""
        self.tick_buff = {}
        self.contract_meta_data = self.__contract_meta__()

    def __contract_meta__(self):
        contract_meta_data = load_metadata_payload("contract")
        if self.contract_meta_data is None:
            raise ValueError(f"Missing Critical Metadata: Contract Meta Not Found.")
        contract_data = contract_meta_data.get("data", None)
        if contract_data is None or (isinstance(contract_data, list) and len(contract_data) == 0):
            raise ValueError(f"Missing Critical Metadata: Contract Meta Not Found.")
        df = pd.DataFrame(contract_data)
        df = df[[
            'ExchangeID', 'InstrumentID', 'InstrumentName', 'ProductClass', 'ProductID', 'VolumeMultiple', 
            'OpenDate', 'ExpireDate', 'UnderlyingInstrID', 'OptionsType', 'StrikePrice', 'InstLifePhase'
        ]].copy()

        df = df.rename(
            columns={
                'ExchangeID': 'exchange', 
                'InstrumentID': 'ctp_contract', 
                'InstrumentName': 'name', 
                'ProductClass': 'product_class', 
                'ProductID': 'symbol', 
                'VolumeMultiple': 'multiplier', 
                'OpenDate': 'list_date', 
                'ExpireDate': 'expiry_date', 
                'UnderlyingInstrID': 'underlying', 
                'OptionsType': 'option_type', 
                'StrikePrice': 'strike', 
                'InstLifePhase': 'status'
            }
        ).set_index('ctp_contract', drop=True)
        df['strike'] = pd.to_numeric(df['strike'], errors='coerce')
        df['multiplier'] = pd.to_numeric(df['multiplier'], errors='coerce')
        return df

    def Run(self) -> None:
        self.api = mdapi.CThostFtdcMdApi.CreateFtdcMdApi()
        self.api.RegisterFront(self.front)
        self.api.RegisterSpi(self)
        self.api.Init()

    def stop(self) -> None:
        self.running = False
        if self.api is not None:
            try:
                self.api.Release()
            except Exception:
                pass

    def OnFrontConnected(self) -> None:
        req = mdapi.CThostFtdcReqUserLoginField()
        if self.username:
            req.UserID = self.username
        if self.password:
            req.Password = self.password
        self.api.ReqUserLogin(req, 0)

    def OnFrontDisconnected(self, nReason: int) -> None:
        log(f"front disconnected: {nReason}")
        self.login_failed = True
        self.failure_reason = f"front disconnected: {nReason}"
        self.stop()

    def OnRspUserLogin(self, pRspUserLogin, pRspInfo, nRequestID: int, bIsLast: bool) -> None:
        if pRspInfo is not None and pRspInfo.ErrorID != 0:
            log(f"login failed: {pRspInfo.ErrorMsg}")
            self.login_failed = True
            self.failure_reason = str(pRspInfo.ErrorMsg)
            self.stop()
            return

        if not self.instruments:
            log("no instruments provided; nothing to subscribe")
            self.login_failed = True
            self.failure_reason = "no instruments provided"
            self.stop()
            return

        self.login_ok = True
        self.api.SubscribeMarketData([item.encode("utf-8") for item in self.instruments], len(self.instruments))
        log(f"subscribed {len(self.instruments)} instruments")

    def OnRtnDepthMarketData(self, pDepthMarketData) -> None:
        tick = {
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
        self.tick_buff[pDepthMarketData.InstrumentID] = tick

    def __raw_md_snapshot__(self):
        return pd.DataFrame.from_dict(self.tick_buff.copy(), orient='index')

    def md(self):
        md_df = self.__raw_md_snapshot__()
        cond = md_df[md_df.dtypes[md_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()] > 1e10
        md_df[
            md_df.dtypes[md_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()
        ] = md_df[
            md_df.dtypes[md_df.dtypes.astype(str).isin(['float64', 'int64'])].index.tolist()
        ].mask(cond, np.nan)
        
        md_df['datetime'] = pd.to_datetime(
            md_df['ActionDay'].astype(str) +
            " " +
            md_df['UpdateTime'] +
            "." +
            md_df['UpdateMillisec'].astype(str)
        )
        md_df = md_df[
            (md_df['datetime'] <= (datetime.now() + timedelta(minutes=2))) &
            (md_df['datetime'] >= (datetime.now() + timedelta(minutes=-2)))
        ].copy()

        md_df = md_df.drop(
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

        contract_df = self.contract_meta_data.copy()
        md_df[[
            'exchange', 'name', 'product_class', 'symbol', 'multiplier', 'list_date', 'expiry_date', 'underlying', 'option_type', 'strike', 'status'
        ]] = contract_df.loc[contract_df.index.intersection(md_df.index)][[
            'exchange', 'name', 'product_class', 'symbol', 'multiplier', 'list_date', 'expiry_date', 'underlying', 'option_type', 'strike', 'status'
        ]].reindex(md_df.index)

        md_df['trading_date'] = pd.to_datetime(md_df['trading_date'])
        md_df['list_date'] = pd.to_datetime(md_df['list_date'])
        md_df['expiry_date'] = pd.to_datetime(md_df['expiry_date'])

        md_df.reset_index(drop=True, inplace=True)
        return md_df


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="starSling live market data (CTP)")
    parser.add_argument("--api", default="ctp", help="API name (default: ctp)")
    parser.add_argument("--protocol", default="tcp", help="Protocol (default: tcp)")
    parser.add_argument("--host", required=True, help="Market data host")
    parser.add_argument("--port", required=True, type=int, help="Market data port")
    parser.add_argument("--username", default="", help="CTP username (optional)")
    parser.add_argument("--password", default="", help="CTP password (optional)")
    parser.add_argument("--instruments", default="", help="Comma-separated instrument list")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.api.lower() != "ctp":
        log("only ctp api is supported in this prototype")
        return 2

    instruments = parse_instruments(args.instruments)
    if not instruments:
        log("no instruments provided")
        return 2

    front = f"{args.protocol}://{args.host}:{args.port}"
    spi = MdSpi(front, instruments, args.username, args.password)

    def handle_signal(signum, frame) -> None:
        spi.stop()

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    log(f"connecting to {front}")
    spi.Run()

    start_ts = time.monotonic()
    exit_code = 0
    while spi.running:
        if not spi.login_ok and (time.monotonic() - start_ts) > LOGIN_TIMEOUT_SECONDS:
            log(f"login timeout after {LOGIN_TIMEOUT_SECONDS}s")
            spi.login_failed = True
            spi.failure_reason = "login timeout"
            spi.stop()
            exit_code = 2
            break
        time.sleep(0.2)

        market_snapshot = spi.md()

    if spi.login_failed:
        exit_code = 2
    log("live md stopped")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())

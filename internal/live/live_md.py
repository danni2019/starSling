import argparse
import json
import signal
import sys
import time

from openctp_ctp import thostmduserapi as mdapi

LOGIN_TIMEOUT_SECONDS = 60

def log(message: str) -> None:
    print(message, file=sys.stderr, flush=True)


def parse_instruments(raw: str) -> list[str]:
    if not raw:
        return []
    return [item.strip() for item in raw.split(",") if item.strip()]


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
            "LastPrice": pDepthMarketData.LastPrice,
            "Volume": pDepthMarketData.Volume,
            "UpdateTime": pDepthMarketData.UpdateTime,
            "UpdateMillisec": pDepthMarketData.UpdateMillisec,
            "BidPrice1": pDepthMarketData.BidPrice1,
            "BidVolume1": pDepthMarketData.BidVolume1,
            "AskPrice1": pDepthMarketData.AskPrice1,
            "AskVolume1": pDepthMarketData.AskVolume1,
        }
        print(json.dumps(tick, separators=(",", ":")), flush=True)


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

    if spi.login_failed:
        exit_code = 2
    log("live md stopped")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())

from __future__ import annotations

import asyncio
import os
from datetime import datetime
import time
from typing import Any, Dict, Optional
import pandas as pd  # type: ignore
import numpy as np  # type: ignore

import logging
from lib.buff.client import RedisClientManager
from lib.buff.config import get_env_settings, get_db_for_live, SEG_LIVE
from lib.buff.keyspace import cache_key
from lib.buff.pubsub import BuffPubSub
from lib.buff.stream import BuffStream
from lib.timez import get_project_tzinfo
from lib.buff.cache import BuffCache
from services.livetrader.src.live_md.openctp_md import CMdImpl
from lib.tool import is_trading


CONFIG_PATH_DEFAULT = "/app/.livetrade"
PUB_TOPIC = "cn.future.ctp_md"
CACHE_NS = "asset"
CACHE_KEY = "cn.future.ctp_md"


def _configure_logging() -> None:
    """Configure root logger to INFO with a simple stream handler.
    Ensures logs are emitted to docker logs (stdout)."""
    fmt = logging.Formatter(
        "%(asctime)s %(levelname)s [%(name)s] %(message)s",
        "%Y-%m-%d %H:%M:%S",
    )
    root = logging.getLogger()
    if not root.handlers:
        h = logging.StreamHandler()
        h.setFormatter(fmt)
        root.addHandler(h)
    else:
        for h in list(root.handlers):
            try:
                h.setFormatter(fmt)
            except Exception:
                continue
    root.setLevel(logging.INFO)


# Configure logging at import time so subsequent loggers inherit settings
_configure_logging()


def _load_config(path: str) -> Dict[str, str]:
    cfg: Dict[str, str] = {}
    if not os.path.exists(path):
        return cfg
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            s = line.strip()
            if not s or s.startswith("#"):
                continue
            if "=" not in s:
                continue
            k, v = s.split("=", 1)
            cfg[k.strip()] = v.strip()
    return cfg


class LiveMdSupervisor:
    def __init__(self, md_front: str, hb_timeout_sec: int, health_port: int) -> None:
        self._log = logging.getLogger("ctp_md")
        self.md_front = md_front
        self.hb_timeout_sec = hb_timeout_sec
        self.health_port = health_port

        self._md: Optional[CMdImpl] = None
        self._last_marker_value: Optional[str] = None
        self._last_marker_seen_ts: float = 0.0
        self._has_seen_marker: bool = False
        self._last_start_ts: float = 0.0
        self._running = False
        self._md_updated: asyncio.Event = asyncio.Event()

        # redis
        settings = get_env_settings()
        self._rcm = RedisClientManager(settings)
        self._pub = BuffPubSub(self._rcm)
        self._cache = BuffCache(self._rcm)
        self._stream = BuffStream(self._rcm)
        self._db_live = get_db_for_live()
        self._frame_seq: int = 0
        # attach cfg dict later in main() for dynamic params
        self.cfg: Dict[str, str] = {}
        # control key for config + presence
        self._ctl_key = cache_key(SEG_LIVE, "ctl", "ctp_md_config")
        # effective control state (defaults = persistent on)
        self._ctl_persistent: bool = True
        self._ctl_linger_sec: int = 300
        self._ctl_heartbeat: int = 15
        self._ctl_presence_expires_at: float = 0.0  # epoch seconds
        self._presence_inactive_since: Optional[float] = None  # monotonic seconds

    def _df_to_records(self, df: pd.DataFrame) -> list[dict[str, Any]]:
        """将 DataFrame 转换为 JSON 可序列化的 records 列表。

        约定：实时行情 DataFrame 中的时间列均为“无时区（naive）”时间。

        - 将所有 datetime 列格式化为字符串 'YYYY-MM-DD HH:MM:SS'
        - 将对象列中可能出现的 Timestamp/datetime 逐个转为字符串
        - 将 NaN/NaT 替换为 None
        - 将 numpy 标量（int64/float64/bool_ 等）转换为原生 Python 类型
        """
        out = df.copy()
        # 1) datetime 列统一为字符串（输入已约定为无时区 naive 时间）
        dt_cols = list(out.select_dtypes(include=["datetime64[ns]"]).columns)
        for c in dt_cols:
            out[c] = out[c].dt.strftime("%Y-%m-%d %H:%M:%S")
        # 2) 对象列中的 Timestamp/datetime → 字符串
        for c in out.columns:
            if out[c].dtype == object:
                out[c] = out[c].apply(
                    lambda x: (
                        x.strftime("%Y-%m-%d %H:%M:%S")
                        if isinstance(x, (pd.Timestamp, datetime))
                        else x
                    )
                )
        # 3) NaN/NaT → None
        out = out.where(pd.notnull(out), None)
        # 4) 转 records 并确保 numpy 标量转换为原生类型
        records = out.to_dict(orient="records")
        py_records: list[dict[str, Any]] = []
        for rec in records:
            py_rec: dict[str, Any] = {}
            for k, v in rec.items():
                if isinstance(v, np.integer):
                    py_rec[k] = int(v)
                elif isinstance(v, np.floating):
                    py_rec[k] = float(v)
                elif isinstance(v, np.bool_):
                    py_rec[k] = bool(v)
                elif isinstance(v, (pd.Timestamp, datetime)):
                    py_rec[k] = v.strftime("%Y-%m-%d %H:%M:%S")
                else:
                    py_rec[k] = v
            py_records.append(py_rec)
        return py_records

    def _update_marker_from_df(self, df) -> None:
        """更新心跳标记：基于 DataFrame 中的最新 datetime 或 datetime_minute。"""
        try:
            if df is None or getattr(df, "empty", True):
                return
            col = "datetime" if "datetime" in df.columns else ("datetime_minute" if "datetime_minute" in df.columns else None)
            if col is None:
                return
            latest = df[col].max()
            marker = str(latest)
            now_ts = asyncio.get_event_loop().time()
            if marker != self._last_marker_value:
                self._last_marker_value = marker
                self._last_marker_seen_ts = now_ts
                self._has_seen_marker = True
        except Exception:
            # 忽略异常，保持上次标记
            pass

    def start_md(self) -> None:
        if self._md is not None:
            return
        self._md = CMdImpl(self.md_front)
        # wire update callback → event-driven publishing
        try:
            loop = asyncio.get_event_loop()
        except RuntimeError:
            loop = None
        if hasattr(self._md, "set_on_md"):
            def _notify() -> None:
                try:
                    if loop is not None:
                        loop.call_soon_threadsafe(self._md_updated.set)
                except Exception:
                    pass
            try:
                self._md.set_on_md(_notify)  # type: ignore[attr-defined]
            except Exception:
                pass
        self._md.Run()
        try:
            self._last_start_ts = asyncio.get_event_loop().time()
        except RuntimeError:
            self._last_start_ts = 0.0

    def stop_md(self) -> None:
        if self._md is None:
            return
        try:
            if getattr(self._md, "api", None) is not None:
                # OPENCTP Api typical shutdown
                self._md.api.Release()  # type: ignore[attr-defined]
        except Exception:
            pass
        finally:
            self._md = None
            # 重置心跳相关状态，避免跨交易时段沿用陈旧标记导致误判停滞
            self._has_seen_marker = False
            self._last_marker_value = None
            self._last_marker_seen_ts = 0.0
            self._last_start_ts = 0.0

    async def _serve_healthz(self) -> None:
        async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
            try:
                data = await reader.read(1024)
                req = data.decode("utf-8", errors="ignore")
                # very small http parser
                if req.startswith("GET /healthz"):
                    resp = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok"
                else:
                    resp = "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
                writer.write(resp.encode("utf-8"))
                await writer.drain()
            finally:
                try:
                    writer.close()
                    await writer.wait_closed()
                except Exception:
                    pass

        server = await asyncio.start_server(handle, host="0.0.0.0", port=self.health_port)
        async with server:
            await server.serve_forever()

    async def _publish_loop(self) -> None:
        seg = SEG_LIVE
        topic = PUB_TOPIC
        ckey = cache_key(seg, CACHE_NS, CACHE_KEY)
        # event-driven: push immediately on md update
        while True:
            try:
                await self._md_updated.wait()
                self._md_updated.clear()
                if self._md is not None:
                    get_df = getattr(self._md, "get_current_md_df", None)
                    df = get_df() if callable(get_df) else None
                    self._update_marker_from_df(df)
                    if df is not None and not getattr(df, "empty", True):
                        # 计算 asof_td：仅允许来自 df['trading_date'] 的最大值
                        if "trading_date" not in df.columns:
                            raise RuntimeError("trading_date column missing for asof_td")
                        try:
                            td = pd.to_datetime(df["trading_date"], errors="coerce").max()
                        except Exception as e:
                            raise RuntimeError("failed to parse trading_date for asof_td") from e
                        if pd.isna(td):
                            raise RuntimeError("invalid trading_date for asof_td")
                        asof_td = str(pd.to_datetime(td).strftime("%Y%m%d"))
                        # 生成 asof_ts（项目时区、秒级）
                        tz = get_project_tzinfo()
                        asof_ts = datetime.now(tz).replace(microsecond=0).isoformat()

                        records = self._df_to_records(df)

                        # 1) KV 最新帧
                        cache_ok = False
                        try:
                            await self._cache.set(self._db_live, ckey, records)
                            cache_ok = True
                        except Exception:
                            cache_ok = False

                        # 2) Stream（裁剪 + 追加）
                        try:
                            try:
                                stream_maxlen = int(self.cfg.get("MD_STREAM_MAXLEN", "600"))  # type: ignore[attr-defined]
                            except Exception:
                                stream_maxlen = 600
                            await self._stream.xadd_with_maxlen(
                                self._db_live,
                                seg,
                                topic,
                                asof_td=asof_td,
                                asof_ts=asof_ts,
                                frame=records,
                                maxlen=stream_maxlen,
                            )
                        except Exception:
                            pass

                        # 3) PubSub（最后、仅在缓存写入成功后发布）
                        if cache_ok:
                            self._frame_seq = (self._frame_seq + 1) % (10**9)
                            payload = {"asof_ts": asof_ts, "asof_td": asof_td, "frame_seq": self._frame_seq}
                            await self._pub.publish(self._db_live, seg, topic, payload)
                        try:
                            self._log.info(
                                f"pub kv/ps/stream live md asof_td={asof_td} rows={getattr(df,'shape',None)}"
                            )
                        except Exception:
                            pass
            except Exception:
                # swallow and continue; tiny backoff avoids hot loop on errors
                await asyncio.sleep(0.05)

    async def _refresh_ctl_config(self) -> None:
        try:
            cfg = await self._cache.get(self._db_live, self._ctl_key)
            if not isinstance(cfg, dict):
                return
            self._ctl_persistent = bool(cfg.get("persistent", True))
            # coerce ranges but keep safe defaults
            try:
                ls = int(cfg.get("linger_sec", 300))
                if ls < 60:
                    ls = 60
                if ls > 600:
                    ls = 600
                self._ctl_linger_sec = ls
            except Exception:
                self._ctl_linger_sec = 300
            try:
                hb = int(cfg.get("heartbeat", 15))
                if hb < 1:
                    hb = 1
                if hb > 30:
                    hb = 30
                self._ctl_heartbeat = hb
            except Exception:
                self._ctl_heartbeat = 15
            try:
                pe = float(cfg.get("presence_expires_at", 0.0))
            except Exception:
                pe = 0.0
            self._ctl_presence_expires_at = pe
        except Exception:
            # ignore read errors; keep previous effective config
            pass

    async def _supervise_loop(self) -> None:
        while True:
            try:
                await self._refresh_ctl_config()
                now_epoch = time.time()
                trading = is_trading()
                # presence gating only applies when persistent is False
                presence_active = True
                if not self._ctl_persistent:
                    presence_active = now_epoch <= self._ctl_presence_expires_at

                if trading and (self._ctl_persistent or presence_active):
                    if self._md is None:
                        # not started → start and allow grace period
                        self.start_md()
                    else:
                        # 优先使用 md_status；并结合心跳标记与宽限期判断
                        md_ok = False
                        try:
                            md_ok = bool(getattr(self._md, "md_status", False))
                        except Exception:
                            md_ok = False
                        now_ts = asyncio.get_event_loop().time()
                        # 宽限期：启动后 hb_timeout_sec 内不做重启判定
                        in_grace = False
                        if self._last_start_ts:
                            in_grace = (now_ts - self._last_start_ts) < self.hb_timeout_sec

                        if not md_ok:
                            if not in_grace:
                                self.stop_md()
                                self.start_md()
                        else:
                            # 已连接但可能停牌/卡死：仅在见过至少一次数据后启用心跳超时判定
                            # 判定逻辑：超过 3 × MD_HEARTBEAT_TIMEOUT_SEC 未见新 marker 才认为停滞
                            if self._has_seen_marker and self._last_marker_seen_ts:
                                if (now_ts - self._last_marker_seen_ts) > (self.hb_timeout_sec * 3) and not in_grace:
                                    self.stop_md()
                                    self.start_md()
                    # reset linger tracking while active
                    self._presence_inactive_since = None
                else:
                    # inactive path: either non-trading or presence off when non-persistent
                    # apply linger timer only when we were previously active and now presence turned off
                    if self._md is not None:
                        do_stop = False
                        if not trading:
                            do_stop = True
                        else:
                            # trading but presence inactive and non-persistent
                            if not self._ctl_persistent and not presence_active:
                                mono_now = asyncio.get_event_loop().time()
                                if self._presence_inactive_since is None:
                                    self._presence_inactive_since = mono_now
                                if (mono_now - self._presence_inactive_since) >= float(self._ctl_linger_sec):
                                    do_stop = True
                            else:
                                self._presence_inactive_since = None
                        if do_stop:
                            self.stop_md()
                await asyncio.sleep(1.0)
            except Exception:
                await asyncio.sleep(1.0)

    async def run(self) -> None:
        self._running = True
        await asyncio.gather(
            self._serve_healthz(),
            self._publish_loop(),
            self._supervise_loop(),
        )


def main() -> None:
    cfg_path = os.getenv("LIVETRADE_CONFIG", CONFIG_PATH_DEFAULT)
    cfg = _load_config(cfg_path)
    md_front = cfg.get("MD_FRONT")
    if not md_front:
        raise RuntimeError("Missing MD_FRONT in .livetrade config")
    hb_sec = int(cfg.get("MD_HEARTBEAT_TIMEOUT_SEC", "10"))
    health_port = int(cfg.get("MD_HEALTH_PORT", "9102"))

    sup = LiveMdSupervisor(md_front=md_front, hb_timeout_sec=hb_sec, health_port=health_port)
    try:
        sup.cfg = cfg  # type: ignore[attr-defined]
    except Exception:
        pass
    asyncio.run(sup.run())


if __name__ == "__main__":
    main()

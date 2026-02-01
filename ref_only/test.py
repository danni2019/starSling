import argparse
import json
import os
import signal
import sys
import time
import pandas as pd
import numpy as np


def log(message: str) -> None:
    print(message, file=sys.stderr, flush=True)


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

def get_contract(x): 
    df = pd.DataFrame(x.get('data'))
    df = df[['ExchangeID', 'InstrumentID', 'InstrumentName', 'ProductClass', 'ProductID', 'VolumeMultiple', 'OpenDate', 'ExpireDate', 'UnderlyingInstrID', 'OptionsType', 'StrikePrice', 'InstLifePhase']]
    
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
    )
    return df


if __name__ == "__main__":
    c = load_metadata_payload("contract")
    cdf = get_contract(c)
    print(cdf)
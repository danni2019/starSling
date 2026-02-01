# Python integration

Live market data runs through an embedded Python 3.11 script that depends on
the OpenCTP Python bindings (`openctp_ctp`). Install the bindings according to
your vendor instructions.

Recommended setup (standalone runtime):

```bash
./scripts/bootstrap_python.sh
```

The bootstrap script downloads a Python 3.11 runtime into `runtime/<platform>/`
and installs requirements into a local venv. If your OpenCTP bindings are a
wheel, set `OPENCTP_WHEEL=/path/to/openctp.whl` before running the script.
You can also set `PIP_INDEX_URL` or `PIP_EXTRA_INDEX_URL` if you use a private
package index.

This directory is reserved for future Python-based market data ingestion and
strategy/model implementations.

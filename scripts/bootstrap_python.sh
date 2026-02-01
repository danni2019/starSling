#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR="${ROOT_DIR}/runtime"
REQ_FILE="${ROOT_DIR}/python/requirements.txt"

PYTHON_VERSION="3.11.9"

if [[ "${STARSLING_PYTHON_VERSION:-}" != "" ]]; then
  PYTHON_VERSION="${STARSLING_PYTHON_VERSION}"
fi

OS="$(uname -s)"
ARCH="$(uname -m)"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required but not found" >&2
  exit 1
fi
if ! command -v tar >/dev/null 2>&1; then
  echo "tar is required but not found" >&2
  exit 1
fi

platform=""
case "${OS}" in
  Darwin)
    if [[ "${ARCH}" == "arm64" ]]; then
      platform="macos-arm64"
    else
      platform="macos-x86_64"
    fi
    ;;
  Linux)
    if [[ "${ARCH}" == "x86_64" ]]; then
      platform="linux-x86_64"
    else
      echo "Unsupported Linux arch: ${ARCH}" >&2
      exit 1
    fi
    ;;
  *)
    echo "Unsupported OS: ${OS}" >&2
    exit 1
    ;;
 esac

PYTHON_DIR="${RUNTIME_DIR}/${platform}/python"
VENV_DIR="${RUNTIME_DIR}/${platform}/venv"

mkdir -p "${PYTHON_DIR}"

python_url=""
case "${platform}" in
  macos-arm64)
    python_url="https://github.com/indygreg/python-build-standalone/releases/download/20240415/cpython-${PYTHON_VERSION}+20240415-aarch64-apple-darwin-install_only.tar.gz"
    ;;
  macos-x86_64)
    python_url="https://github.com/indygreg/python-build-standalone/releases/download/20240415/cpython-${PYTHON_VERSION}+20240415-x86_64-apple-darwin-install_only.tar.gz"
    ;;
  linux-x86_64)
    python_url="https://github.com/indygreg/python-build-standalone/releases/download/20240415/cpython-${PYTHON_VERSION}+20240415-x86_64-unknown-linux-gnu-install_only.tar.gz"
    ;;
 esac

if [[ ! -x "${PYTHON_DIR}/bin/python3" ]]; then
  echo "Downloading Python ${PYTHON_VERSION} for ${platform}..."
  tmpfile=$(mktemp)
  curl -fsSL "${python_url}" -o "${tmpfile}"
  tar -xzf "${tmpfile}" -C "${PYTHON_DIR}" --strip-components=1
  rm -f "${tmpfile}"
fi

"${PYTHON_DIR}/bin/python3" -m venv "${VENV_DIR}"
"${VENV_DIR}/bin/pip" install --upgrade pip

PIP_ARGS=()
if [[ "${PIP_INDEX_URL:-}" != "" ]]; then
  PIP_ARGS+=("--index-url" "${PIP_INDEX_URL}")
fi
if [[ "${PIP_EXTRA_INDEX_URL:-}" != "" ]]; then
  PIP_ARGS+=("--extra-index-url" "${PIP_EXTRA_INDEX_URL}")
fi

if [[ "${OPENCTP_WHEEL:-}" != "" ]]; then
  if [[ ${#PIP_ARGS[@]} -gt 0 ]]; then
    "${VENV_DIR}/bin/pip" install "${OPENCTP_WHEEL}" "${PIP_ARGS[@]}"
    "${VENV_DIR}/bin/pip" install -r "${REQ_FILE}" "${PIP_ARGS[@]}"
  else
    "${VENV_DIR}/bin/pip" install "${OPENCTP_WHEEL}"
    "${VENV_DIR}/bin/pip" install -r "${REQ_FILE}"
  fi
else
  if [[ ${#PIP_ARGS[@]} -gt 0 ]]; then
    "${VENV_DIR}/bin/pip" install -r "${REQ_FILE}" "${PIP_ARGS[@]}"
  else
    "${VENV_DIR}/bin/pip" install -r "${REQ_FILE}"
  fi
fi

cat <<INFO
Python runtime ready at:
  ${VENV_DIR}/bin/python
INFO

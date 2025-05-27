#!/bin/sh

SCRIPT=$(readlink -f "$0")
ROOT=$(unset CDPATH && cd "$(dirname "$SCRIPT")/.." && pwd)

if ! command -v ginkgo >/dev/null 2>&1; then
    echo "ginkgo not found, try to install..."
    go install github.com/onsi/ginkgo/v2/ginkgo@v2.23.4
fi

debug=false
if [ "x${DEBUG}" = "xtrue" ]; then
    debug=true
fi
logLevel=info
if [ "${LOG_LEVEL}" ]; then
    logLevel="${LOG_LEVEL}"
fi

fftPath=${ROOT}/bin/fft
if [ "${FFT_PATH}" ]; then
    fftPath="${FFT_PATH}"
fi
fftsPath=${ROOT}/bin/ffts
if [ "${FFTS_PATH}" ]; then
    fftsPath="${FFTS_PATH}"
fi
fftwPath=${ROOT}/bin/fftw
if [ "${FFTW_PATH}" ]; then
    fftwPath="${FFTW_PATH}"
fi
concurrency="4"
if [ "${CONCURRENCY}" ]; then
    concurrency="${CONCURRENCY}"
fi

echo "Building fft binaries..."
make -C ${ROOT} build

echo "Running e2e tests..."
ginkgo -nodes=${concurrency} --poll-progress-after=60s ${ROOT}/test/e2e -- -fft-path=${fftPath} -ffts-path=${fftsPath} -fftw-path=${fftwPath} -log-level=${logLevel} -debug=${debug}

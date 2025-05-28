all: fmt build

build: fft fftw ffts bandwidth-test

fmt:
	go fmt ./...

fft:
	go build -ldflags "-s -w" -o bin/fft ./cmd/fft

fftw:
	go build -ldflags "-s -w" -o bin/fftw ./cmd/fftw

ffts:
	go build -ldflags "-s -w" -o bin/ffts ./cmd/ffts

bandwidth-test:
	go build -ldflags "-s -w" -o bin/bandwidth-test ./cmd/bandwidth-test

e2e:
	./hack/run-e2e.sh

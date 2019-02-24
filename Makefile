all: fmt build

build: fft fftw ffts

fmt:
	go fmt ./...

fft:
	go build -ldflags "-s -w" -o bin/fft ./cmd/fft

fftw:
	go build -ldflags "-s -w" -o bin/fftw ./cmd/fftw

ffts:
	go build -ldflags "-s -w" -o bin/ffts ./cmd/ffts

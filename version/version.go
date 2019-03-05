package version

var version string = "0.0.1"

func Full() string {
	return version
}

var defaultServerAddr string = "fft.gofrp.org:7777"

func DefaultServerAddr() string {
	return defaultServerAddr
}

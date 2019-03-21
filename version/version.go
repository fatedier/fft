package version

var version string = "0.2.0"

func Full() string {
	return version
}

var defaultServerAddr string = "fft.gofrp.org:7777"

func DefaultServerAddr() string {
	return defaultServerAddr
}

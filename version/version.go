// Package version holds build information. Release builds set the variables
// with -ldflags "-X github.com/extimsu/urlchecker/version.Version=...".
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the release version, without the leading "v".
	Version = "dev"
	// GitCommit is the commit the binary was built from.
	GitCommit = "unknown"
	// BuildDate is the build time in RFC 3339.
	BuildDate = "unknown"
)

// App prints the build information.
func App() {
	fmt.Println("Version:", Version)
	fmt.Println("Git Commit:", GitCommit)
	fmt.Println("Build Date:", BuildDate)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Printf("OS / Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

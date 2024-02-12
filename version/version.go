package version

import (
	"fmt"
	"runtime"
)

// GitCommit returns the git commit that was compiled. This will be filled in by the compiler.
var GitCommit string

// Version returns the main version number that is being run at the moment.
const Version = "0.1.2"

// BuildDate returns the date the binary was built
var BuildDate = ""

// GoVersion returns the version of the go runtime used to compile the binary
var GoVersion = runtime.Version()

// OsArch returns the os and arch used to build the binary
var OsArch = fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)

var Author = fmt.Sprint("Alexander Baulin, 2024 (c) https://extim.su")

func App() {
	fmt.Println("Build Date:", BuildDate)
	fmt.Println("Git Commit:", GitCommit)
	fmt.Println("Version:", Version)
	fmt.Println("Go Version:", GoVersion)
	fmt.Println("OS / Arch:", OsArch)
	fmt.Println("Author:", Author)
}

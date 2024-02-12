package help

import "fmt"

// ShowHelp - showing short help
func Show() {
	fmt.Println(`
	_____________
	< URL-checker >
	 -------------
	`)
	fmt.Println("Usage: urlchecker --url <url>")
	fmt.Println("")
	fmt.Println("urlchecker --url <url> --port <port>")
	fmt.Println("urlchecker --file <filename>")
	fmt.Println("")
	fmt.Println("For more information try --help")
}

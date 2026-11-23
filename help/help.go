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
	fmt.Println("urlchecker --metrics --metrics-port <port> --check-interval <duration>")
	fmt.Println("urlchecker --exporter --workers <count> --check-interval <duration> (includes metrics)")
	fmt.Println("urlchecker --exporter --url icmp://<host> --ping-interval <duration> (ICMP packet loss)")
	fmt.Println("")
	fmt.Println("For more information try --help")
}

package monitor

import (
	"godvr/internal/dvrip"
)

func main() {
	conn, err := dvrip.New()
	if err != nil {
		panic(err)
	}

	conn.Login()
	conn.Monitor()

}

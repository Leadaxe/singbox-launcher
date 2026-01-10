package main

import (
	"fmt"
	"singbox-launcher/core/config/subscription"
)

func main() {
	uri := `vless://01335b71-f287-4f95-811a-eadd69261590@188.132.232.183:443?type=tcp&security=reality&encryption=none&flow=xtls-rprx-vision&fp=chrome&sni=gitlab.com&sid=&pbk=MG2dYVNcCz8Q0wo5YXAywRqfR9d92ui_d86S-Ekmz0A#7205d096-c620-4394-9a44-bb18baa3a988@papervpn.io&prefix=%16%03%01%00%C2%A8%01%01#PaperVPN_TR`
	node, err := subscription.ParseNode(uri, nil)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	if node == nil {
		fmt.Println("Node is nil (skipped)")
		return
	}
	fmt.Printf("Parsed node: %+v\n", node)
}

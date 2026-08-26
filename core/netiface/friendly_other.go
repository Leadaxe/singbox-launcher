//go:build !darwin

package netiface

// friendlyName: на не-darwin системное имя интерфейса уже человекочитаемо
// (Linux: eth0/wlan0; Windows: net.Interface.Name отдаёт имя соединения вида
// «Ethernet» / «Wi-Fi»), отдельный источник подписей не нужен.
func friendlyName(string) string { return "" }

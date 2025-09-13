package main

import (
	"flag"
	"log"
	"net"

	"vpn/utils"

	"github.com/songgao/water"
)

func main() {

	serverAddress := flag.String("server", "", "Remote VPN server IP and port")
	flag.Parse()
	if *serverAddress == "" {
		log.Fatal("Error: you must provide the server address with -server flag")
	}

	tun := utils.CreateTUN("10.0.5.1", "10.0.5.2", "utun5")
	log.Println("Interface Name:", tun.Name())

	socketClient(tun, *serverAddress)
}

func socketClient(incomingTun *water.Interface, serverAddress string) {
	conn, err := net.Dial("udp", serverAddress)
	if err != nil {
		log.Fatal("Error opening socket.", err)
		return
	}
	log.Println("UDP socket opened to:", serverAddress)
	defer conn.Close()

	go readFromTunToUdp(incomingTun, conn)
	go readFromUdpToTun(incomingTun, conn)

	select {}
}

func readFromTunToUdp(tun *water.Interface, conn net.Conn) {
	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			log.Printf("Error reading from %s: %v\n", tun.Name(), err)
			continue
		}

		packet := utils.ParseIPv4(buf[:n])
		log.Printf("Read from %s: %+v", tun.Name(), packet)

		if _, err := conn.Write(buf[:n]); err != nil {
			log.Printf("Error writing to %s: %v", conn.RemoteAddr(), err)
			continue
		}
		log.Printf("Wrote to %s via UDP.", conn.RemoteAddr())
	}
}

func readFromUdpToTun(tun *water.Interface, conn net.Conn) {
	buf := make([]byte, 65535)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Println("read error:", err)
			continue
		}
		packet := utils.ParseIPv4(buf[:n])
		log.Printf("Will write to %s: %+v", tun.Name(), packet)

		if _, err := tun.Write(buf[:n]); err != nil {
			log.Println("error writing to TUN:", err)
		}
		log.Printf("%s: Wrote %d bytes: %+v", tun.Name(), n, buf[:n])
	}
}

package main

import (
	"log"
	"net"
	"sync/atomic"
	"vpn/utils"

	"github.com/songgao/water"
)

func main() {

	tun := utils.CreateTUN("10.0.6.1", "10.0.6.2", "utun6")
	log.Println("Interface Name:", tun.Name())

	go socketServer(tun)

	select {}
}

func socketServer(incomingTun *water.Interface) {
	addr, err := net.ResolveUDPAddr("udp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	log.Println("Listening on: ", addr)

	var remoteAddress atomic.Value

	go readFromUdpToTun(conn, incomingTun, &remoteAddress)
	go readFromTunToUdp(incomingTun, conn, &remoteAddress)

	select {}
}

func readFromUdpToTun(conn *net.UDPConn, tun *water.Interface, remoteAddr *atomic.Value) {
	buf := make([]byte, 65535)
	for {
		n, clientAddress, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("read error:", err)
			continue
		}
		packet := utils.ParseIPv4(buf[:n])
		log.Printf("Read from UDP: %+v", packet)

		remoteAddr.Store(clientAddress)
		if _, err := tun.Write(buf[:n]); err != nil {
			log.Println("error writing to TUN:", err)
		}
		log.Printf("Wrote to %s: %+v", tun.Name(), packet)
	}
}

func readFromTunToUdp(tun *water.Interface, conn *net.UDPConn, remoteAddr *atomic.Value) {
	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			log.Printf("Error reading from %s: %v\n", tun.Name(), err)
			continue
		}

		packet := utils.ParseIPv4(buf[:n])
		log.Printf("Read from %s: %+v", tun.Name(), packet)

		if clientAddress, ok := remoteAddr.Load().(*net.UDPAddr); ok && clientAddress != nil {
			if _, err := conn.WriteToUDP(buf[:n], clientAddress); err != nil {
				log.Printf("Error writing to %s: %v", clientAddress, err)
				continue
			}
			log.Printf("Wrote back to %s via UDP.", clientAddress)
		}
	}
}

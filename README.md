# Writing your own VPN implementation in Go

As a project to improve my computer networking skills and continue my Go learning journey, I decided to peek behind the
curtain of a technology that has been around for a long time: **Virtual Private Networks (VPNs)**.

These overlay networks, which provide a tunnel from server A to server B, are a great way to hide your internet traffic
from prying eyes and to ensure encryption.

Go is a language perfectly suited for networking development, making it an ideal candidate for this project. Before
diving into the actual implementation, however, I feel it’s necessary to explain what VPNs are and what different kinds
of protocols exist. Afterwards, we’ll build our own minimal VPN implementation using only Go’s standard library plus the
`songgao/water` package for working with TUN devices.

___

## Virtual Private Networks

As a project to improve my computer networking skills and continue my Go learning journey, I decided to peek behind the
curtain of a technology that has been around for a long time: **Virtual Private Networks (VPNs)**.

These overlay networks, which provide a tunnel from server A to server B, are a great way to hide your internet traffic
from prying eyes and to ensure encryption.

Go is a language perfectly suited for networking development, making it an ideal candidate for this project. Before
diving into the actual implementation, however, I feel it’s necessary to explain what VPNs are and what different kinds
of protocols exist. Afterwards, we’ll build our own minimal VPN implementation using only Go’s standard library plus the
`songgao/water` package for working with TUN devices.

___

## Existing VPN protocols - OpenVPN and Wireguard

Of course, the explanation above is a gross simplification and omits many important details. Let’s look one level deeper
at how different VPN protocols handle packet routing (we’ll leave encryption aside for now to keep the scope
manageable).

There are numerous VPN protocols, but the most popular and widely used ones are **OpenVPN**, **IPSec**, and **WireGuard
**. In this article, I’ll focus exclusively on OpenVPN and WireGuard, as they are built quite differently and help
illustrate the fundamentals of tunneling protocols.

The key distinction between them lies in **which OSI layer they operate on** and **how they process packets**.

- **OpenVPN** supports both Layer 2 (L2) and Layer 3 (L3) tunneling.
- **WireGuard** is more opinionated and supports only Layer 3 tunneling.

OpenVPN uses **TUN/TAP devices** (explained below), while WireGuard operates within the kernel itself.

___

### TUN/TAP Devices

TUN/TAP devices are virtual networking interfaces that allow you to handle low-level networking operations in user
space, providing access to raw IP packets.

- **TUN** devices operate at **Layer 3 (Network Layer)** — they handle IP packets.
- **TAP** devices operate at **Layer 2 (Data Link Layer)** — they handle Ethernet frames.

In this article, we’ll focus only on TUN devices.

OpenVPN can use either TUN or TAP devices. WireGuard, on the other hand, is implemented as a kernel module and does not
use user-space TUN devices by default. Its implementation can be found here:  
[https://github.com/torvalds/linux/tree/master/drivers/net/wireguard](https://github.com/torvalds/linux/tree/master/drivers/net/wireguard).

When you set up a WireGuard interface, it registers a new kernel-level network device that behaves like any other (e.g.,
`eth0`, `lo`). This approach offers major performance advantages, since packets never have to leave the kernel to be
processed in user space.

There’s also a Go-based user-space implementation of WireGuard for non-Linux platforms:  
[https://github.com/WireGuard/wireguard-go](https://github.com/WireGuard/wireguard-go).

For a detailed visualization of OpenVPN’s packet flow, see this excellent diagram:  
[https://community.openvpn.net/Pages/HowPacketsFlow](https://community.openvpn.net/Pages/HowPacketsFlow).

___

## Packet Flow with TUN Devices

Since our implementation will also use TUN devices, let’s walk through an example packet flow from client to
destination, using real IPs and device names to keep it tangible.

**Client:**

```
eth0: 192.168.178.20
default gateway: 192.168.178.1
TUN5: 10.0.5.1
```

**Server:**

```
Public IP: 167.71.55.250
TUN6: 10.0.6.1
```

On the client, we first define routing rules so all outgoing packets are routed via `tun5`:

```bash
route -q -n add -inet 0.0.0.0/1 -interface utun5
route -q -n add -inet 128.0.0.0/1 -interface utun5
```

With this setup, all traffic is directed to the TUN interface. From there, in user space, we can access the raw TCP/IP
packets. We establish a UDP connection from the client to the VPN server, and for every packet received on `tun5`, we
send
it through this UDP socket to the remote server. Thus, the original packet becomes the payload of a UDP/IP packet.

The server listens on the same UDP socket. When it receives a packet, the OS has already processed the UDP/IP layer,
leaving us with the original IP packet, which we write to `tun6`. The kernel then routes it out through `eth0` to its
final
destination (e.g., `domain-name-resolver.fly.dev`).

Responses from `domain-name-resolver.fly.dev` travel back to the server, which routes packets destined for `10.0.5.1` to
the client’s tunnel interface. Here we need
to define another important routing
rule. Below script ensures that all packets coming to our server that have 10.0.5.1 as the destination address will be
sent to the tun6 interface.

```bash
ip route replace 10.0.5.1/32 dev utun6
```

We can visualize this with logs. Suppose the client makes a request to https://domain-name-resolver.fly.dev.

**Client logs:**

```
2025/10/06 11:20:05 Read from utun5: {Version:4 ... SrcAddr:10.0.5.1 DestAddr:66.241.124.31}
2025/10/06 11:20:05 Wrote to 167.71.35.250:8080 via UDP.
```

**Server logs:**

```
2025/10/06 09:17:03 Read from UDP: {Version:4 ... DestAddr:66.241.124.31}
2025/10/06 09:17:03 Wrote to utun6: {Version:4 ... DestAddr:66.241.124.31}
2025/10/06 09:17:03 Read from utun6: {Version:4 ... SrcAddr:66.241.124.31 DestAddr:10.0.5.1}
2025/10/06 09:17:03 Wrote back to client socket via UDP.
```

On the way back, the client reads the UDP data, writes it to tun5, and the kernel routes it back to the original TCP
socket connection — completing the round trip.

<figure id="figure-2">
  <img src="diagrams/tunnel.svg" alt="Detailed flow">
  <figcaption>Figure 2: Detailed flow of a packet in a VPN tunnel with TUN devices</figcaption>
</figure>

___

## Translating the theory into code

Go is an excellent language for networking — it strikes a great balance between simplicity and low-level control. The
client and server code are nearly identical; for brevity, we’ll look only at the client.

**Create the TUN interface:**

```go
package main

func main() {
	tun := utils.CreateTUN("10.0.5.1", "10.0.5.2", "utun5")
	log.Println("Interface Name:", tun.Name())

	socketClient(tun)
}
```

**Set up the UDP socket and manage packet flow:**

```go
package main

func socketClient(incomingTun *water.Interface) {
	conn, err := net.Dial("udp", "167.71.35.250:8080")
	if err != nil {
		log.Fatal("Error opening socket.", err)
		return
	}
	defer conn.Close()

	go readFromTunToUdp(incomingTun, conn)
	go readFromUdpToTun(incomingTun, conn)

	select {}
}
```

**Forward packets from TUN → UDP:**

```go
package main

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
```

**Forward packets from UDP → TUN:**

```go
package main

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
```

With just these few functions, we have a minimal working VPN client. The server implementation mirrors this closely.


## Running the VPN

### Client
From the project root directory, run the following commands:

```bash
sudo go run ./client -server <SERVER_IP>:<SERVER_PORT>
cd setup
sudo ./clientRouteConfig.sh
```

### Server
From the project root directory, run the following commands:
```bash
sudo go run ./server
cd setup
sudo ./serverRouteConfig.sh
```

## Next steps

This is, of course, a very naive VPN implementation — it lacks encryption, authentication, and connection management.
The next logical step would be to add encryption, one of the core features that makes VPNs secure and private.



# Useful Links

* https://www.wireguard.com/papers/wireguard.pdf
* https://community.openvpn.net/Pages/HowPacketsFlow.
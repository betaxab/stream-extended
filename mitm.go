package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

func checkAllowDomain(name string) bool {
	for i := 0; i < len(Data.Domain); i++ {
		if strings.HasSuffix(name, Data.Domain[i]) {
			return true
		}
	}

	return false
}

func startHTTP() {
	for {
		log.Printf("[APP][HTTP] %v", beginHTTP())

		time.Sleep(time.Second * 4)
	}
}

func beginHTTP() error {
	ln, err := net.Listen("tcp", "127.0.0.1:80")
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Println("[APP][HTTP] Started")

	for {
		client, err := ln.Accept()
		if err != nil {
			if errno, ok := err.(net.Error); ok {
				if errno.Temporary() {
					continue
				}
			}

			return err
		}

		go handleHTTP(client)
	}
}

func handleHTTP(client net.Conn) {
	defer client.Close()

	data := make([]byte, 1400)
	size, err := client.Read(data)
	if err != nil {
		return
	}
	data = data[:size]

	list := make(map[string]string)

	{
		hdr := bytes.Split(bytes.Split(data, []byte{0x0d, 0x0a, 0x0d, 0x0a})[0], []byte{0x0d, 0x0a})
		for i := 0; i < len(hdr); i++ {
			text := string(hdr[i])

			if i == 0 {
				continue
			}

			SPL := strings.SplitN(text, ":", 2)
			list[strings.ToUpper(strings.TrimSpace(SPL[0]))] = strings.TrimSpace(SPL[1])
		}
	}

	if domain, ok := list["HOST"]; ok {
		if !checkAllowDomain(domain) {
			remote, err := net.Dial("tcp", net.JoinHostPort(list["HOST"], "80"))
			if err != nil {
				return
			}
			defer remote.Close()

			if _, err := remote.Write(data); err != nil {
				return
			}
			data = nil

			Pipe(client, remote)
			return
		}
	} else {
		return
	}

	log.Printf("[APP][HTTP] %s <-> %s", client.RemoteAddr(), list["HOST"])

	remote, err := net.Dial("tcp", Data.Remote[0])
	if err != nil {
		return
	}
	defer remote.Close()

	if _, err := remote.Write(data); err != nil {
		return
	}
	data = nil

	Pipe(client, remote)
}

func startTLS() {
	for {
		log.Printf("[APP][TLS] %v", beginTLS())

		time.Sleep(time.Second * 4)
	}
}

func beginTLS() error {
	ln, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Println("[APP][TLS] Started")

	for {
		client, err := ln.Accept()
		if err != nil {
			if errno, ok := err.(net.Error); ok {
				if errno.Temporary() {
					continue
				}
			}

			return err
		}

		go handleTLS(client)
	}
}

func handleTLS(client net.Conn) {
	defer client.Close()

	data := make([]byte, 1400)
	size, err := client.Read(data)
	if err != nil {
		return
	}
	data = data[:size]

	if data[0] != 0x16 {
		return
	}

	offset := 0
	offset += 1 // Content Type
	offset += 2 // Version
	offset += 2 // Length

	// Handshake Type
	if data[offset] != 0x01 {
		log.Printf("[APP][TLS][%s] Not Client Hello", client.RemoteAddr())
		return
	}
	offset += 1

	offset += 3  // Length
	offset += 2  // Version
	offset += 32 // Random

	// Session ID
	length := int(data[offset])
	offset += 1
	offset += length

	// Cipher Suites
	length = (int(data[offset]) << 8) + int(data[offset+1])
	offset += 2
	offset += length

	// Compression Methods
	length = int(data[offset])
	offset += 1
	offset += length

	// Extension Length
	offset += 2

	domain := ""
	for offset < size && domain == "" {
		// Extension Type
		name := (int(data[offset]) << 8) + int(data[offset+1])
		offset += 2

		// Extension Length
		length = (int(data[offset]) << 8) + int(data[offset+1])
		offset += 2

		// Extension: Server Name
		if name == 0 {
			// Server Name List Length
			offset += 2

			// Server Name Type
			if data[offset] != 0x00 {
				log.Printf("[APP][TLS][%s] Not Host Name", client.RemoteAddr())
				return
			}
			offset += 1

			// Server Name Length
			length = (int(data[offset]) << 8) + int(data[offset+1])
			offset += 2

			// Server Name
			domain = string(data[offset : offset+length])

			// Get Out
			break
		}

		// Extension Data
		offset += length
	}

	if !checkAllowDomain(domain) {
		remote, err := net.Dial("tcp", net.JoinHostPort(domain, "443"))
		if err != nil {
			return
		}
		defer remote.Close()

		if _, err := remote.Write(data); err != nil {
			return
		}
		data = nil

		Pipe(client, remote)
		return
	}

	log.Printf("[APP][TLS] %s <-> %s", client.RemoteAddr(), domain)

	remote, err := net.Dial("tcp", Data.Remote[1])
	if err != nil {
		return
	}
	defer remote.Close()

	if _, err := remote.Write(data); err != nil {
		return
	}
	data = nil

	Pipe(client, remote)
}

func Pipe(client net.Conn, remote net.Conn) {
	go func() {
		_, _ = io.CopyBuffer(remote, client, make([]byte, 1400))
		_ = client.SetDeadline(time.Now())
		_ = remote.SetDeadline(time.Now())
	}()

	_, _ = io.CopyBuffer(client, remote, make([]byte, 1400))
	_ = client.SetDeadline(time.Now())
	_ = remote.SetDeadline(time.Now())
}
package client

import (
	"bufio"
	"bytes"
	"io"
	"net"

	"crypto/md5"
	"encoding/base64"
)

// Gives the "IPID" hash for the address. The purpose of this is so
// clients' IPs aren't leaked to moderators. It intends to be a unique identifier
// for each IP.
func hashIP(addr net.Addr) string {
	// We only accept TCP connections, so this is safe.
	ip := addr.(*net.TCPAddr).IP.String()

	// We use MD5 to hash the IP, then base64 it.
	// This results in about 25-26 characters. We use the last 6.
	// Each base64 character is 6 bits, so we end up with 36 bits, or about
	// 68,719,476,736 unique hashes. This *might* be good enough.
	h := md5.New()
	io.WriteString(h, ip)
	enc := base64.RawStdEncoding.EncodeToString(h.Sum(nil))
	return enc[len(enc)-6:]
}

// Splits data read at every occurrence of `char`.
func splitAt(char byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// No more data to return.
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		// Found `char`.
		if i := bytes.Index(data, []byte{char}); i != -1 {
			return i + 1, data[0:i], nil
		}
		// Reached EOF - return rest of data.
		if atEOF {
			return len(data), data, nil
		}
		// Wait for more data.
		return 0, nil, nil
	}
}

package pubip

import (
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

type PublicIP struct {
	IP           net.IP
	SourceFilter *SourceFilter
}

func getStringFromBitmaskMap(m map[uint]string, p uint) string {
	pstr := ""
	for i := uint(0); (1 << i) <= uint(p); i++ {
		if p&(1<<i) != 0 {
			if pstr != "" {
				pstr += "|"
			}
			if s, ok := m[(1 << i)]; ok {
				pstr += s
			} else {
				pstr += strconv.FormatUint(uint64(1<<i), 10)
			}
		}

	}
	return pstr
}

type Protocol int

const (
	HTTP Protocol = (1 << iota)
	HTTPS
	DNS
	DNS_TXT
)

func (p Protocol) String() string {
	m := map[uint]string{
		uint(HTTP):    "HTTP",
		uint(HTTPS):   "HTTPS",
		uint(DNS):     "DNS",
		uint(DNS_TXT): "DNS (TXT)",
	}

	return getStringFromBitmaskMap(m, uint(p))
}

type FamilyReply int

const (
	IPv4 FamilyReply = (1 << iota)
	IPv6
)

func (fr FamilyReply) String() string {
	m := map[uint]string{
		uint(IPv4): "IPv4",
		uint(IPv6): "IPv6",
	}

	return getStringFromBitmaskMap(m, uint(fr))

}

type SourceID int

const (
	ICANHAZIP SourceID = (1 << iota)
	IFCONFIGME
	IPIFY
	OPENDNS
	GOOGLEDNSTXT
)

func (sid SourceID) String() string {
	m := map[uint]string{
		uint(ICANHAZIP):    "ICANHAZIP",
		uint(IFCONFIGME):   "IFCONFIGME",
		uint(IPIFY):        "IPIFY",
		uint(OPENDNS):      "OPENDNS",
		uint(GOOGLEDNSTXT): "GOOGLEDNSTXT",
	}

	return getStringFromBitmaskMap(m, uint(sid))
}

type SourceFilter struct {
	ID      SourceID
	Proto   Protocol
	Replies FamilyReply
}

func NewSourceFilter() *SourceFilter {
	return &SourceFilter{}
}

type Source struct {
	ID          SourceID
	Address     string
	AddressIPv6 string
	AddressIPv4 string
	Server      string
	Protocols   Protocol
	Replies     FamilyReply
}

func (s *Source) Fetch() (net.IP, bool) {
	if s.Protocols&HTTPS != 0 {
		url := "https://" + s.Address
		log.Println("Fetching from", url)
		if ip, ok := stringToNetIP(getHTTPBody(url)); ok {
			return ip, ok
		}
	}
	if s.Protocols&HTTP != 0 {
		url := "http://" + s.Address
		log.Println("Fetching from", url)
		if ip, ok := stringToNetIP(getHTTPBody(url)); ok {
			return ip, ok
		}
	}
	if s.Protocols&DNS != 0 || s.Protocols&DNS_TXT != 0 {
		log.Println("Fetching from", s.Server)
		ip, err := getDNSQuery(s.Address, s.Server)
		if err == nil {
			return ip, true
		} else {
			log.Println(err)
		}
	}

	// TODO: implent other protocol methods

	return nil, false
}

func Get(sf *SourceFilter) (net.IP, bool) {
	sourcesList := map[SourceID]Source{
		IFCONFIGME: Source{
			ID:        IFCONFIGME,
			Address:   "ifconfig.me/ip",
			Protocols: HTTP | HTTPS,
			Replies:   IPv4,
		},
		ICANHAZIP: Source{
			ID:          ICANHAZIP,
			Address:     "icanhazip.com",
			AddressIPv4: "ipv4.icanhazip.com",
			AddressIPv6: "ipv6.icanhazip.com",
			Protocols:   HTTP | HTTPS,
			Replies:     IPv4 | IPv6,
		},
		IPIFY: Source{
			ID:          IPIFY,
			Address:     "api.ipify.org",
			AddressIPv4: "api.ipify.org",
			AddressIPv6: "api64.ipify.org",
			Protocols:   HTTP | HTTPS,
			Replies:     IPv4 | IPv6,
		},
		GOOGLEDNSTXT: Source{
			ID:        GOOGLEDNSTXT,
			Address:   "o-o.myaddr.l.google.com",
			Server:    "ns1.google.com",
			Protocols: DNS_TXT,
			Replies:   IPv4 | IPv6,
		},
		OPENDNS: Source{
			ID:        OPENDNS,
			Address:   "myip.opendns.com",
			Server:    "resolver1.opendns.com",
			Protocols: DNS,
			Replies:   IPv4 | IPv6,
		},
	}

	for _, s := range getValidSources(sf, sourcesList) {
		if ip, ok := s.Fetch(); ok {
			return ip, ok
		}
	}

	return nil, false
}

func getValidSources(sf *SourceFilter, sourcesList map[SourceID]Source) map[SourceID]Source {
	validSources := make(map[SourceID]Source)

	if sf == nil {
		return sourcesList
	}

	for k, v := range sourcesList {
		if sf.Proto != 0 && v.Protocols&sf.Proto == 0 {
			continue
		}
		if sf.Replies != 0 && v.Replies&sf.Replies == 0 {
			continue
		}
		if sf.ID != 0 && v.ID&sf.ID == 0 {
			continue
		}

		// current v apparently matches atleast partially on all set filter properties
		// create a new source containing only the matched properties
		s := Source{ID: v.ID, Address: v.Address}
		s.Server = v.Server
		if sf.Proto != 0 {
			s.Protocols = v.Protocols & sf.Proto
		} else {
			s.Protocols = v.Protocols
		}
		if sf.Replies != 0 {
			s.Replies = v.Replies & sf.Replies

			if s.Replies == IPv4 && v.AddressIPv4 != "" {
				s.Address = v.AddressIPv4
			} else if s.Replies == IPv6 && v.AddressIPv6 != "" {
				s.Address = v.AddressIPv6
			}
		} else {
			s.Replies = v.Replies
		}

		// now add the filtered source to the valid list
		log.Println("Adding valid source:", s)
		validSources[k] = s

	}

	return validSources
}

func New() *PublicIP {

	pip := NewEmpty()

	pip.Update()

	return pip
}

func NewEmpty() *PublicIP {
	var pip PublicIP
	return &pip
}

func (pip *PublicIP) Update() bool {
	if ip, ok := Get(pip.SourceFilter); ok {
		pip.IP = ip
		return true
	}
	return false
}

func stringToNetIP(ipstring string, err error) (net.IP, bool) {
	if err != nil {
		return nil, false
	}

	ip := net.ParseIP(ipstring)
	if ip != nil {
		return ip, true
	}

	return nil, false
}

func getHTTPBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		//fmt.Println(err)
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		//fmt.Println(err)
		return "", err
	}

	return strings.TrimSpace(string(body)), nil

}

func getDNSQuery(addr string, server string) (net.IP, error) {
	serverport := server

	m := new(dns.Msg)

	m.SetQuestion(dns.Fqdn(addr), dns.TypeANY)

	if addrs, err := net.LookupHost(server); err == nil {
		// FIXME: we should really try all addresses.
		serverport = net.JoinHostPort(addrs[0], "53")
	} else {
		serverport = server
	}

	// FIXME: this will not work for ipv6 addresses.
	if strings.LastIndex(serverport, ":") < 0 {
		serverport = net.JoinHostPort(serverport, "53")
	}

	log.Println("Doing dns-query against", serverport)

	c := new(dns.Client)
	in, _, err := c.Exchange(m, serverport)

	if err != nil {
		return nil, err
	}

	if len(in.Answer) <= 0 {
		return nil, errors.New("Not enough content in answer in reply")
	}

	if t, ok := in.Answer[0].(*dns.A); ok {
		return t.A, nil
	}

	if t, ok := in.Answer[0].(*dns.AAAA); ok {
		return t.AAAA, nil
	}

	if t, ok := in.Answer[0].(*dns.TXT); ok {
		var text strings.Builder
		for i := 0; i < len(t.Txt); i++ {
			text.WriteString(t.Txt[i])
		}
		if ip := net.ParseIP(text.String()); ip != nil {
			return ip, nil
		}
	}

	return nil, errors.New("Failed to get A or AAAA reply")
}

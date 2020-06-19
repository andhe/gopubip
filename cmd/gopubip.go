package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	pubip "github.com/andhe/gopubip"
)

func main() {
	pip := pubip.NewEmpty()

	// Parse command line options and set up pip.SourceFilter
	ipv4 := flag.Bool("4", false, "Filter on IPv4")
	ipv6 := flag.Bool("6", false, "Filter on IPv6")

	var id string
	flag.StringVar(&id, "i", "", "id of a specific provider to use.")

	flag.Parse()

	if *ipv4 {
		if pip.SourceFilter == nil {
			pip.SourceFilter = pubip.NewSourceFilter()
		}
		pip.SourceFilter.Replies |= pubip.IPv4
	}

	if *ipv6 {
		if pip.SourceFilter == nil {
			pip.SourceFilter = pubip.NewSourceFilter()
		}
		pip.SourceFilter.Replies |= pubip.IPv6
	}

	// Note: if map used in SourceID -> String() was made public this could be prettier.
	for sid := pubip.SourceID(1 << 0); sid.String() != strconv.Itoa(int(sid)); sid <<= 1 {
		if strings.ToUpper(id) == strings.ToUpper(sid.String()) {
			if pip.SourceFilter == nil {
				pip.SourceFilter = pubip.NewSourceFilter()
			}
			pip.SourceFilter.ID = sid
			break
		}
	}

	// Finally do actual query and print result on success
	if pip.Update() {
		fmt.Println(pip.IP)
	} else {
		log.Fatal("Failed to get public ip-address.")
	}
}

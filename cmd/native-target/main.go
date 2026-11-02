package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/build/native"

	"github.com/ygelfand/LANovo/internal/board"
)

// native-target [board] prints the native API one board's helpers build for;
// native-target -all prints every API lanovod carries helpers for.
func main() {
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: native-target [-all | board]")
		os.Exit(1)
	}
	var targets []native.Target
	for _, b := range board.All() {
		targets = append(targets, native.Target{Name: b.Name, API: b.NativeAPI})
	}

	if len(os.Args) == 2 && os.Args[1] == "-all" {
		var apis []string
		for _, t := range targets {
			api, err := native.Select(targets, t.Name)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if s := strconv.Itoa(api); !slices.Contains(apis, s) {
				apis = append(apis, s)
			}
		}
		slices.Sort(apis)
		fmt.Println(strings.Join(apis, " "))
		return
	}

	name := ""
	if len(os.Args) == 2 {
		name = os.Args[1]
	}
	api, err := native.Select(targets, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(api)
}

package main

import "os"

var status = 1

func main() {
	os.Exit(0)       // want "direct call to os.Exit in main is forbidden"
	(os.Exit)(1)     // want "direct call to os.Exit in main is forbidden"
	(os.Exit)(1)     // want "direct call to os.Exit in main is forbidden"
	defer os.Exit(1) // want "direct call to os.Exit in main is forbidden"
	go os.Exit(1)    // want "direct call to os.Exit in main is forbidden"
	if status > 0 {
		os.Exit(1) // want "direct call to os.Exit in main is forbidden"
	}
	for i := 0; i < status; i++ {
		os.Exit(1) // want "direct call to os.Exit in main is forbidden"
	}
	if func() bool { os.Exit(1); return true }() {
		os.Exit(1) // want "direct call to os.Exit in main is forbidden"
	}
	func() { os.Exit(1) }()
	defer func() { os.Exit(1) }()
	go func() { os.Exit(1) }()
	exit := os.Exit
	exit(1)
}

func helper() { os.Exit(1) }

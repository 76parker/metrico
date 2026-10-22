package main

import system "os"

func main() {
	system.Exit(0)         // want "direct call to os.Exit in main is forbidden"
	defer (system.Exit)(0) // want "direct call to os.Exit in main is forbidden"
	go (system.Exit)(0)    // want "direct call to os.Exit in main is forbidden"
}

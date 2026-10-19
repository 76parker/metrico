package main

import . "os"

func main() {
	Exit(0)         // want "direct call to os.Exit in main is forbidden"
	defer (Exit)(0) // want "direct call to os.Exit in main is forbidden"
	go (Exit)(0)    // want "direct call to os.Exit in main is forbidden"
	func() { Exit(1) }()
}

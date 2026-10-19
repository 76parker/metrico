package main

import "os"

type application struct{}

func (application) main() { os.Exit(1) }

func helper() { os.Exit(1) }

func main() {}

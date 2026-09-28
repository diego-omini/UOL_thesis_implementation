//go:build bindings

package main

// This file prevents runtime persistence initialization during Wails binding discovery.

const generatingBindings = true

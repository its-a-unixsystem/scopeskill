package main

import "testing"

func TestParseGlobalFlagsConfig(t *testing.T) {
	configPath, args, err := parseGlobalFlags([]string{"--config", "/tmp/scopevisio.env", "get", "/myaccount"})
	if err != nil {
		t.Fatal(err)
	}
	if configPath != "/tmp/scopevisio.env" {
		t.Fatalf("config path = %q", configPath)
	}
	if len(args) != 2 || args[0] != "get" || args[1] != "/myaccount" {
		t.Fatalf("args = %#v", args)
	}
}

package main

import "testing"

func TestDarkPalette(t *testing.T) {
	if !chromeTheme.Dark || chromeTheme.Text == chromeTheme.Background {
		t.Fatal("invalid dark theme")
	}
	if codeTheme.Background != chromeTheme.Background {
		t.Fatal("editor background mismatch")
	}
}

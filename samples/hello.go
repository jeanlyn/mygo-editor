// A small file for trying tabs, highlighting, and search.
package main

import "fmt"

type Project struct {
	Name  string
	Files []string
}

func main() {
	project := Project{
		Name:  "MyGo Editor",
		Files: []string{"hello.go", "config.json"},
	}

	for index, name := range project.Files {
		fmt.Printf("%d. %s\n", index+1, name)
	}
}

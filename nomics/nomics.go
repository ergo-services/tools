package main

import (
	"fmt"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var k = koanf.New(".")

func main() {

	k.Load(file.Provider("mock/mock.yml"), yaml.Parser())
	fmt.Println("parent's name is = ", k.String("parent1.name"))
}

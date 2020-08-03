package main

import (
	"fmt"
	"strings"
)

func main() {
	str1 := "01-cpu"
	str2 := "023-tt"

	fmt.Println(strings.Index(str1, "-"))
	fmt.Println(strings.Index(str2, "-"))
	if strings.Index(str1, "-") == 2{
		fmt.Println(str1[3:])
	}

}

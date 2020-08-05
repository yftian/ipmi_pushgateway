package main

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"os/exec"
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
	str3 := "M/B_Inlet_Temp"
	str4 := strings.ReplaceAll(str3, "/","")
	fmt.Println(str4)


	cmd := exec.Command("cmd")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		return
	}
	fmt.Println(out.String, "jieguo", errors.New(stderr.String()))
	fmt.Println(errors.New(stderr.String()), errors.New(stderr.String())==nil)

}

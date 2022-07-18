package main

import (
	"fmt"
	"testing"
)

func TestRandomNum(t *testing.T) {
	for i := 0; i < 5; i++ {
		nums := generateRandomNumber(10, 30, 10)
		fmt.Println(nums)
	}
}

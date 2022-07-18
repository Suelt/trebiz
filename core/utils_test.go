package core

import (
	"fmt"
	"testing"
)

func TestRandomNumber(t *testing.T) {

	for i := 0; i < 5; i++ {
		num := getRandomNum()
		fmt.Println(num)
	}
}

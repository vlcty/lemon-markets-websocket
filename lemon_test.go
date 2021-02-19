package lemon

import (
	"testing"
	"time"
)

func TestIsExchangeOpen(t *testing.T) {
	location, _ := time.LoadLocation("Europe/Berlin")

	testCases := map[time.Time]bool{
		// Thursday
		time.Date(2021, time.February, 19, 23, 15, 0, 0, location): false,
		time.Date(2021, time.February, 19, 7, 25, 0, 0, location):  false,
		time.Date(2021, time.February, 19, 7, 31, 0, 0, location):  true,
		time.Date(2021, time.February, 19, 12, 15, 0, 0, location): true,
		// Saturday (10:00 - 13:00)
		// 09:10 -> false
		time.Date(2021, time.February, 20, 9, 10, 0, 0, location): false,
		// 10:30 -> true
		time.Date(2021, time.February, 20, 10, 30, 0, 0, location): true,
		// Edgecase: One second after opening
		time.Date(2021, time.February, 20, 10, 0, 1, 0, location): true,
	}

	counter := 0

	for thetime, expected := range testCases {
		result := isExchangeOpen(thetime)

		if result != expected {
			t.Fatalf("Test case #%d failed. Expected: %t, Result: %t", counter, expected, result)
		}

		counter++
	}
}

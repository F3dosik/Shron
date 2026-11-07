package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxGroupLength — наибольшая длина названия группы в символах.
const MaxGroupLength = 64

// ErrGroupTooLong возвращается, если название группы длиннее MaxGroupLength.
var ErrGroupTooLong = fmt.Errorf("название группы длиннее %d символов", MaxGroupLength)

// NormalizeGroup приводит название группы к виду, в котором оно хранится: пробелы по
// краям убираются, подряд идущие пробелы и переводы строк заменяются одним пробелом,
// чтобы «Работа» и « Работа » не стали разными группами. Пустая строка — без группы.
func NormalizeGroup(s string) (string, error) {
	group := strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(group) > MaxGroupLength {
		return "", ErrGroupTooLong
	}
	return group, nil
}

package command

import (
	"errors"
	"fmt"
	"os"

	"github.com/F3dosik/Shron/internal/client/passgen"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/spf13/cobra"
)

// genFlags — параметры генератора паролей в командной строке.
type genFlags struct {
	length      int
	noLower     bool
	noUpper     bool
	noDigits    bool
	noSymbols   bool
	noAmbiguous bool
}

// register добавляет флаги параметров пароля.
func (f *genFlags) register(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.length, "length", passgen.DefaultLength, "длина пароля")
	cmd.Flags().BoolVar(&f.noLower, "no-lower", false, "без строчных букв")
	cmd.Flags().BoolVar(&f.noUpper, "no-upper", false, "без заглавных букв")
	cmd.Flags().BoolVar(&f.noDigits, "no-digits", false, "без цифр")
	cmd.Flags().BoolVar(&f.noSymbols, "no-symbols", false, "без спецсимволов (если сайт их не принимает)")
	cmd.Flags().BoolVar(&f.noAmbiguous, "no-ambiguous", false, "без похожих символов 0/O, 1/l/I (для ввода вручную)")
}

// options переводит флаги в параметры генератора.
func (f *genFlags) options() passgen.Options {
	return passgen.Options{
		Length:      f.length,
		Lower:       !f.noLower,
		Upper:       !f.noUpper,
		Digits:      !f.noDigits,
		Symbols:     !f.noSymbols,
		NoAmbiguous: f.noAmbiguous,
	}
}

// newGenerateCmd создаёт команду генерации пароля или парольной фразы.
// В stdout выводится только сам пароль, чтобы его можно было передать дальше
// (например, `shron generate | xclip`); оценка стойкости — в stderr.
func (c *Commands) newGenerateCmd() *cobra.Command {
	var (
		gen       genFlags
		words     int
		separator string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Сгенерировать стойкий пароль или парольную фразу",
		Long: "Генерирует пароль из случайных символов или, с --words, парольную фразу из случайных\n" +
			"слов словаря EFF (7776 слов, около 12,9 бита на слово). Фразу легче запомнить и ввести —\n" +
			"она хорошо подходит для мастер-пароля: 6 слов дают около 77 бит.\n\n" +
			"Примеры:\n" +
			"  shron generate                     # 20 символов\n" +
			"  shron generate --length 32 --no-symbols\n" +
			"  shron generate --words 6           # парольная фраза",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				result passgen.Result
				err    error
			)
			if words > 0 {
				result, err = passgen.Passphrase(words, separator)
			} else {
				result, err = passgen.Password(gen.options())
			}
			if err != nil {
				return err
			}
			fmt.Println(result.Password)
			fmt.Fprintf(os.Stderr, "Надёжность: %.0f бит (%s)\n", result.EntropyBits, passgen.Strength(result.EntropyBits))
			return nil
		},
	}
	gen.register(cmd)
	cmd.Flags().IntVar(&words, "words", 0, fmt.Sprintf("парольная фраза из N слов (%d–%d) вместо пароля из символов",
		passgen.MinWords, passgen.MaxWords))
	cmd.Flags().StringVar(&separator, "separator", "-", "разделитель слов парольной фразы")
	return cmd
}

// errGenerateNotCredentials — --generate указан для типа без пароля.
var errGenerateNotCredentials = errors.New("--generate работает только для типа credentials")

// generatedPassword возвращает сгенерированный пароль, если указан --generate,
// иначе пустую строку.
func generatedPassword(enabled bool, t domain.SecretType, gen genFlags) (string, error) {
	if !enabled {
		return "", nil
	}
	if t != domain.SecretTypeCredentials {
		return "", errGenerateNotCredentials
	}
	result, err := passgen.Password(gen.options())
	if err != nil {
		return "", err
	}
	return result.Password, nil
}

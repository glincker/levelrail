package importplan

import "testing"

func FuzzParseDockerRun(f *testing.F) {
	f.Add(`docker run -d -p 8080:80 -v data:/var/lib/x -e A=b --name web nginx:1`)
	f.Add(`docker run --memory 512m "unterminated`)
	f.Add(`docker run -p 99999999:1 -v :: img`)
	f.Add("")
	f.Fuzz(func(_ *testing.T, text string) {
		_, _ = parseDockerRun(text)
	})
}

func FuzzTokenizeAndClassify(f *testing.F) {
	f.Add(`a "b c" 'd' \ e`)
	f.Add("https://github.com/x/y")
	f.Add("services:\n  a:\n    image: b\n")
	f.Add("FROM scratch\n")
	f.Fuzz(func(_ *testing.T, text string) {
		_, _ = Tokenize(text)
		_, _ = Classify(Input{Text: text})
		_ = ParseEnvFile(text, "fuzz")
	})
}

package implv2

type Action struct {
	Op      string
	Content string
}

func OutputParser(content string) Action

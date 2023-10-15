package slack_threads

func CoalesceStr(inputs ...string) string {
	for _, v := range inputs {
		if v != "" {
			return v
		}
	}
	return ""
}

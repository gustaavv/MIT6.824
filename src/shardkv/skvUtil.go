package shardkv

func cloneStr2StrMap(m map[string]string) map[string]string {
	ans := make(map[string]string)
	for k, v := range m {
		ans[k] = v
	}
	return ans
}

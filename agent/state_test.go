package main

import "testing"

// TestCountTopLevelSites 站点计数:全局块不计,注释/引号/嵌套块不误判。
func TestCountTopLevelSites(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"空配置", "", 0},
		{"仅全局块", "{\n\tadmin localhost:2019\n}\n", 0},
		{"单站点", "a.example.com {\n\trespond \"ok\"\n}\n", 1},
		{"双站点含注释", "# a comment\na.com {\n\trespond 200 # inline {\n}\nb.com {\n\tfile_server\n}\n", 2},
		{"多域名站点", "a.com, b.com {\n\tredir https://c.com\n}\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countTopLevelSites(tc.in); got != tc.want {
				t.Fatalf("countTopLevelSites(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestTrimComment 注释裁剪保留引号内 #。
func TestTrimComment(t *testing.T) {
	if got := trimComment(`respond "ok # not comment" # real`); got != `respond "ok # not comment"` {
		t.Fatalf("trimComment 结果不符: %q", got)
	}
}

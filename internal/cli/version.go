package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// 编译时注入的版本信息
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// BuildInfo 构建信息
type BuildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// NewVersionCmd 创建 version 命令
func NewVersionCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := BuildInfo{
				Version:   Version,
				GitCommit: GitCommit,
				BuildTime: BuildTime,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
			}

			if jsonOutput {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("mote %s\n", info.Version)
				fmt.Printf("  Git commit: %s\n", info.GitCommit)
				fmt.Printf("  Built:      %s\n", info.BuildTime)
				fmt.Printf("  Go version: %s\n", info.GoVersion)
				fmt.Printf("  OS/Arch:    %s/%s\n", info.OS, info.Arch)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")

	return cmd
}

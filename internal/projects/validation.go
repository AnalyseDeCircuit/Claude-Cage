package projects

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sylvinus/agent-vm/internal/mounts"
)

// ValidateWorkspace resolves the directory and excludes host control data.
func ValidateWorkspace(path, profile string) (string, error) {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[2:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("项目路径无效")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.New("项目目录不存在或无法访问")
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", errors.New("请选择一个项目文件夹")
	}
	realProfile, err := filepath.EvalSymlinks(profile)
	if err != nil {
		realProfile = profile
	}
	contains := func(parent, child string) bool {
		rel, e := filepath.Rel(parent, child)
		return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if executable, e := os.Executable(); e == nil {
		if real, e := filepath.EvalSymlinks(executable); e == nil && contains(abs, real) {
			return "", errors.New("不能共享正在运行的工作台程序所在目录。请将安装版 Cage.app 放在项目外，并重启控制服务后选择此项目。")
		}
	}
	if mounts.UnderName(filepath.ToSlash(abs), nil) {
		return "", errors.New("请选择项目根目录，不要共享 Git 等版本控制元数据目录")
	}
	if contains(abs, realProfile) || contains(realProfile, abs) || abs == home || abs == "/" {
		return "", errors.New("请选择具体项目，不要共享主目录、Cage 配置或包含配置的上级目录")
	}
	return abs, nil
}

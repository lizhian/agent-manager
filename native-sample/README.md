# AGENTS.md 原生验证样本（#2）

仅验证 MyGo 原生绘制路径。此目录独立于现有 Tauri 应用，不迁移业务、不访问真实数据库、受管文件或用户文档。40 个片段均在内存；关闭或重置后丢弃。Markdown 外部链接禁止打开，图片不加载。

版本锁定：Go 1.27.3、MyGo v0.7.3（go.mod / go.sum）。需要各平台 MyGo 原生构建所需的系统开发工具；macOS 使用 Xcode Command Line Tools。无需 Node、Rust 或 WebView。

在本目录运行：

```sh
go run .
go run . --minimum # 960×640；默认1180×760
go test ./...
go vet ./...
go build -o /tmp/agent-manager-native-sample .
```

示范：选中片段，编辑名称和正文；开关控制组合预览；切换底部预览或拖动分隔条；拖动专用手柄到目标行（蓝线表示插入该行之前），末尾空白区表示追加。长列表使用框架滚动容器的拖拽边缘滚动。搜索期间禁用排序，清空后恢复。重置弹窗使用 MyGo 的焦点管理与 Escape 取消。

`go test` 只检查内存操作与无窗口控件交互，不能替代原生运行、系统剪贴板、中文输入法组合输入、跨平台焦点或 GPU 渲染验证。详细证据由集成分支保存；没有三平台实际运行证据时 #2 保持未完成，禁止扩大迁移。

待实际验证：中文输入法组合输入、选区、复制粘贴与撤销；Tab/Enter/Space/Escape、弹窗返回焦点；默认/最小窗口布局与截图；手柄插入位置及长列表边缘滚动；Markdown 实际语料兼容；各平台构建反馈。当前实现没有修改框架分支、渲染或输入系统。

图标来源：[Lucide grip-vertical](https://github.com/lucide-icons/lucide/blob/main/icons/grip-vertical.svg)，许可见 assets/lucide-LICENSE。

# README 图标导航设计

## 目标

在保留现有 AstraStoreXion Logo 和文档内容的前提下，提高根目录 `README.md` 的章节可扫描性与页内导航效率。

## 方案

采用“顶部快捷导航 + 章节 Emoji”的组合：

- 在项目简介之后加入一行居中的快捷导航。
- 快捷导航覆盖 README 的八个主要章节。
- 每个导航项由 Emoji、章节名称和显式页内锚点组成。
- 对应章节标题使用相同 Emoji，形成稳定的视觉映射。
- 使用显式英文 `id`，不依赖 GitHub 对中文和 Emoji 标题自动生成的锚点。

## 图标与锚点映射

| 锚点 | 导航文字 | 章节标题 |
| --- | --- | --- |
| `features` | `✨ 当前能力` | `## ✨ 当前能力` |
| `quick-start` | `🚀 快速开始` | `## 🚀 快速开始` |
| `python-sdk` | `🐍 Python SDK` | `## 🐍 Python SDK` |
| `production` | `🛠️ 生产部署` | `## 🛠️ 生产部署` |
| `test-assets` | `🧪 测试资料` | `## 🧪 测试资料` |
| `verification` | `✅ 验证` | `## ✅ 验证` |
| `limitations` | `⚠️ 限制` | `## ⚠️ 限制` |
| `license` | `📄 License` | `## 📄 License` |

两个三级标题也增加图标，但不进入顶部快捷导航：

- `### 🔁 可选主从异步复制`
- `### 📦 可恢复分片上传`

## 快捷导航结构

使用居中 HTML 段落，每个导航项使用仓库内页锚点，并以中点分隔：

```html
<p align="center">
  <a href="#features">✨ 当前能力</a> ·
  <a href="#quick-start">🚀 快速开始</a> ·
  <a href="#python-sdk">🐍 Python SDK</a> ·
  <a href="#production">🛠️ 生产部署</a> ·
  <a href="#test-assets">🧪 测试资料</a> ·
  <a href="#verification">✅ 验证</a> ·
  <a href="#limitations">⚠️ 限制</a> ·
  <a href="#license">📄 License</a>
</p>
```

每个二级章节前放置对应的显式锚点，例如：

```html
<a id="features"></a>

## ✨ 当前能力
```

## 约束

- 保留现有 Logo、标题、正文、代码示例和链接内容。
- 不添加外部图标库、远程图片或依赖。
- 不给代码块中的注释标题添加 Emoji。
- 不添加徽章墙或重复目录。
- 仅修改根目录 `README.md`。

## 验收

- 八个快捷导航链接分别指向八个唯一的显式锚点。
- 快捷导航与对应章节使用相同图标和文字。
- 两个三级功能章节具有清晰且不重复的图标。
- README 原有内容除标题文本外保持不变。
- Markdown/HTML 结构无空白错误。

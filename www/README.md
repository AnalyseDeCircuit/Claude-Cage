# Cage 项目主页

主页地址为 <https://lee-xeo.github.io/Claude-Cage/>，面向首次使用者介绍产品、公开安装包、开始使用步骤与使用边界。

在本目录执行：

```sh
npm ci
npm run dev      # http://localhost:4822/Claude-Cage/
npm run check
npm run build    # dist/
```

默认中文入口与其他语言路由位于 `src/pages/`，共用 `src/components/CageHome.astro`。`src/i18n/cage.ts` 从根目录主 `README.md` 和 `docs/readme/` 中的译文提取正文，`src/i18n/languages.ts` 定义下拉框中的 15 种语言及界面短文案。样式位于 `src/styles/cage.css`，工作台截图位于 `public/`，README 共用素材位于根目录 `assets/`。`astro.config.mjs` 的 `site` 和 `base` 分别指定站点域名与项目子路径；内部链接、图片和重定向使用 `import.meta.env.BASE_URL`。自定义部署可通过 `SITE_URL` 和 `SITE_BASE` 覆盖。

下载信息来自 `src/data/release.json`，表示可公开下载的版本，可以低于根目录 `VERSION` 所标记的开发版本。从仓库根目录运行 `node www/scripts/sync-release.mjs`，即可从匿名 GitHub API 刷新公开版本，并核对附件与校验清单。普通本地构建使用已保存的数据，不要求联网。

GitHub Actions 在 `main` 的网站文件变化、Release 发布或编辑、手动运行时重新构建并部署 GitHub Pages；PR 只检查。Release 事件会先下载实际安装包与对应 Mihomo 源码，验证摘要、Developer ID 签名、Apple 公证和 Gatekeeper，失败时不会部署这一轮网站。发布操作见[公开发行与主页部署](../docs/public-releases.md)。

根目录主 README 是维护者确认产品措辞的来源，其他语言译文集中在 `docs/readme/`。主 README 和译文修改均触发主页重新构建；发布工作流只刷新下载版本并读取现有译文，不自动生成翻译。中文主页为 `/`，其他语言使用 `/<语言代码>/`，语言范围与根目录 README 导航一致，不提供英语。下拉框切换独立 URL；阿拉伯语、波斯语、普什图语、达里语使用从右向左布局。网页正文直接复用对应 README 译文，结构调整时构建会提示维护者同步映射。

GitHub Pages 是当前主页的发布目标，部署配置由仓库工作流维护。

`public/install.sh` 仍被上游 `scripts/tools/upstream/release.sh` 与安装器测试使用，保留用于 agent-vm 兼容流程，不作为 Cage 安装入口。

• 已读取 PROJECT_GUIDE.md，并按项目结构查看了首页模板、主题 CSS、栅格系统和 main.go 的首页数据。当前未修改代码。

  改造目标

  把当前两栏首页改成类似 yelog.org / 3-hexo 的三段式博客布局：左侧个人与导航，中间文章流，右侧辅助信息。yelog 相关主题说
  明里也明确它是 “three-stage design / 三段式设计”，左半边偏博主信息，中间有主页、所有文章、标签云、关于我等导航入口。参
  考来源：yelog.org 和其 3-hexo 主题介绍。
  来源：https://yelog.org/ (https://yelog.org/)、https://github.com/yelog/hexo-theme-3-hexo
  (https://github.com/yelog/hexo-theme-3-hexo)

  现状判断

  项目首页相关文件主要是：

  - templates/frontend/index.html：首页模板，当前结构是 #main col-8 + #secondary col-4
  - usr/themes/default/style.css：主样式，控制头部、文章卡片、侧栏组件、响应式
  - usr/themes/default/grid.css：Bento 栅格，当前桌面 .col-8 实际为 70%，.col-4 实际为 30%
  - usr/themes/default/go-custom.css：少量补充样式
  - main.go：PageData 已有 Posts、RecentPosts、Categories、RecentComments、DateArchives、分页等字段

  当前已有数据足够支持一个基础三栏首页：左栏可放站点信息、分类导航、搜索；中栏放文章列表；右栏放最新文章、最新评论、日期
  归档。若要更像 yelog 的“标签云/关于我/统计”体验，则需要在 main.go 给 PageData 补充 Tags 或站点统计数据。

  推荐方案

  采用“模板重排 + CSS Grid 覆盖旧 float 栅格”的方式，不建议继续依赖现有 grid.css 的百分比列宽。原因是当前 .col-8 =
  70%、.col-4 = 30%，没有合适的三栏比例；直接新增 .col-2/.col-7/.col-3 会被旧 float 和响应式隐藏规则牵制。首页独立使用
  CSS Grid 更清晰，改动范围也更可控。

  桌面布局

  首页主体改为：

  .site-shell
  ├── left sidebar   220px-260px
  ├── main content   minmax(0, 1fr)
  └── right sidebar  260px-300px

  建议宽度：

  grid-template-columns: 240px minmax(0, 1fr) 280px;
  max-width: 1280px;
  gap: 18px;

  中栏仍是文章列表，保持 article.post 逻辑不变。左栏负责“站点身份 + 主导航”，右栏负责“内容辅助”。

  模板调整

  templates/frontend/index.html 建议改成三块语义结构：

  <div class="home-layout">
    <aside class="home-left" role="complementary">...</aside>
    <main class="home-main" id="main" role="main">...</main>
    <aside class="home-right" role="complementary">...</aside>
  </div>

  左栏模块建议：

  - 站点名：.Site.Title
  - 站点描述：.Site.Description
  - 首页入口：/blog
  - 分类列表：.Categories
  - 搜索框：从当前 header 中迁移过来
  - 主题切换按钮：可保留在顶部，也可移到左栏底部

  中栏模块：

  - ArchiveTitle
  - Posts 循环
  - 分页组件

  右栏模块：

  - 最新文章：.RecentPosts
  - 最近评论：.RecentComments
  - 日期归档：.DateArchives
  - 可选：分类统计、标签云、友情链接等

  头部处理

  当前 header 占了搜索、分类导航、主题切换。三栏布局里建议将 header 简化，否则顶部导航和左栏导航会重复。

  推荐改法：

  - 桌面端：header 只保留一个轻量顶栏，例如站点名、主题按钮
  - 左栏承担主要导航
  - 移动端：继续用 .mobile-menu-toggle，但菜单内容从左栏导航或 #nav-menu 克隆

  如果想尽量少动 JS，可以保留隐藏的 #nav-menu，让 mobile-nav.js 继续工作；桌面端视觉上把导航呈现在左栏。

  CSS 改造点

  在 usr/themes/default/style.css 增加首页专用布局类：

  - .home-layout
  - .home-left
  - .home-main
  - .home-right
  - .profile-card
  - .side-nav
  - .sidebar-section
  - .home-left-inner
  - .home-right-inner

  并覆盖旧列系统对首页的影响：

  .home-layout {
    display: grid;
    grid-template-columns: 240px minmax(0, 1fr) 280px;
  }

  .home-layout > * {
    float: none;
    width: auto;
  }

  为了更接近 yelog 的三段式观感，左栏可以使用固定或 sticky：

  .home-left-inner,
  .home-right-inner {
    position: sticky;
    top: 16px;
  }

  但不建议全高 fixed，因为当前页面已有 header/footer 和移动端菜单逻辑，fixed 容易引入滚动冲突。

  响应式策略

  建议分三档：

  - >= 1100px：完整三栏
  - 768px - 1099px：两栏，中栏 + 右栏，左栏导航收进顶部/移动菜单
  - < 768px：单栏，只显示文章流，侧栏模块按顺序折叠到文章后或隐藏

  示意：

  Desktop:  左栏 | 文章 | 右栏
  Tablet:   文章 | 右栏
  Mobile:   文章，侧栏模块下沉或隐藏

  移动端建议优先保留：

  - 搜索
  - 分类导航
  - 文章列表
  - 分页

  最新评论、日期归档可以放在文章列表之后，避免首屏太拥挤。

  是否需要改后端

  基础三栏不需要改 main.go，因为已有字段够用。

  如果要更像 yelog，建议补充两个轻量后端字段：

  - Tags []Tag：首页标签云
  - Stats SiteStats：文章数、分类数、评论数

  对应新增函数：

  - getTags(db)：从 typecho_metas 查询 type='tag'
  - getSiteStats(db)：统计已发布文章、分类、评论数量

  这部分风险低，但属于后端改造；如果第一阶段只做布局，可以先不加。

  分阶段实施计划

  1. 第一阶段：只改首页结构
     调整 templates/frontend/index.html，把现有两栏拆成左/中/右三栏，不改变后端数据。

  2. 第二阶段：新增首页 CSS
     在 style.css 增加 .home-layout 系列样式，保留现有文章卡片、widget 样式，避免大规模重写。

  3. 第三阶段：处理响应式
     确保桌面三栏、平板两栏、手机单栏；检查当前 kit-hidden-tb 不再误隐藏右栏。

  4. 第四阶段：可选增强
     增加标签云、站点统计、关于我模块，需要小幅扩展 PageData 和查询函数。

  5. 第五阶段：验证
     不用 go test ./...。按项目说明验证：
      - go build -o blog_app main.go
      - 运行前台后检查 /blog
      - 检查首页、分类页、搜索页、归档页分页是否仍正常

  建议落地范围

  第一版我建议只改这两个文件：

  - templates/frontend/index.html
  - usr/themes/default/style.css

  先不动 main.go。这样可以最快得到三栏效果，并且不影响数据库查询、分页、评论、归档等业务逻辑。后续确认视觉方向后，再补标
  签云和统计模块。

---

按你刚才的第一版方案执行。

要求：
1. 只修改 templates/frontend/index.html 和 usr/themes/default/style.css
2. 不修改 main.go
3. 不修改数据库
4. 不修改后台
5. 保留现有文章列表、分页、分类、搜索、最新评论、归档功能
6. 做成桌面三栏、平板两栏、手机单栏
7. 修改完成后告诉我改了哪些文件和如何验证

---

检查本次 diff，确认是否只修改了 index.html 和 style.css

---

go build -o blog_app main.go

---






# 生产部署与仓库的差异（2026-09-26 记录）

修「文件管理页容量显示错误」时发现：**服务器 `/opt/blog_li` 的代码落后于 GitHub `main`。**

## 结论（已修正）

第一版报告写反了方向。当时用 `find` 收集文件时，服务器侧把 macOS 元数据垃圾
（80 个 `._*.py`）也算了进去，我因此误判"服务器更新"。改用**语义比对**（对每个
文件解析 AST 后比哈希，排除 `._*` 与 `__pycache__`）后结论明确：

| 项 | 结果 |
| --- | --- |
| 两边共有的非迁移 Python 文件 | 66 个，**AST 完全一致** |
| 差异文件 | 2 个，均为**服务器落后** |
| 仓库缺失、仅服务器存在的文件 | 5 个真实文件 + `__init__.py` 扫描噪声 |

两个落后文件与修复：

| 文件 | 服务器（旧，有问题） | 仓库 `main`（新，正确） |
| --- | --- | --- |
| `apps/upload/views.py` | `def get(self, request, pk)` 配 `public_token` 路由 → **必然 500** | `def get(self, request, token)` + `get_object_or_404(..., public_token=token)` |
| `apps/comment/serializers.py` | 缺 `is_unread` 字段 | 含 `is_unread` + `get_is_unread` |

两个文件已从 `main` 恢复到服务器（部署前备份在
`/var/backups/astrastore-xion/blog-li-*.py.<timestamp>`），公开文件预览从 500 恢复为 200。

## 那次 500 的成因

`blog/urls.py` 把路由改成 token 形式后，视图签名没有同步跟上：

```python
# blog/urls.py:94
path('api/files/public/<str:token>/', PublicFileDownloadView.as_view(), ...)

# 服务器上的旧视图
def get(self, request, pk):
    file_obj = get_object_or_404(UploadFile, pk=pk, is_public=True)
```

Django 以 `token=` 关键字调用，视图只接受 `pk` → `TypeError` → 500。由于列表接口
返回的 `file_url` 已指向该路由，**所有公开文件的缩略图、预览与下载都受影响**，
不只是图片。数据库侧 `upload_file.public_token` 列存在且 20/20 有值，迁移
`0006_move_public_file_links_to_api_files` 已应用，所以问题纯在代码版本。

## 仍未收敛的部分

**5 个文件只存在于服务器，仓库里没有**：

```
apps/category/admin.py
apps/comment/admin.py
apps/dashboard/admin.py
apps/dashboard/models.py
apps/dynamic/admin.py
```

它们是 Django admin 注册与一个 models 文件。目前不影响运行，但意味着**用 `main`
全量重建服务器会丢掉它们**。建议从服务器取回并提交。

## 建议的收敛步骤

1. 把上面 5 个文件从服务器取回仓库并提交（以服务器为准，因为它们只在那里）。
2. 在服务器上 `git init` + 关联远端，或把 `/opt/blog_li` 换成 `git clone`，让运行
   代码始终可追溯到某个 commit。
3. 给 `blog_li` 加发布流水线（构建 → scp → 原子替换 → 健康检查 → 失败回滚），
   参照 AstraStoreXion 的 `.github/workflows/release.yml`。
4. 清理服务器上的 macOS 垃圾：

   ```bash
   cd /opt/blog_li && find . -name '._*' -delete
   ```

## 本次相关改动的落点

- `GET /api/upload/files/summary/`：仓库 commit `057351d`，并已部署到服务器。
- 前端容量显示：`myblog-admin` commit `d4b7935`，已通过流水线发布。
- 公开文件预览 500：由本次恢复 `main` 版本代码修复，服务器上为手工部署。

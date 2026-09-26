# 生产部署与仓库的差异（2026-09-26 记录）

修「文件管理页容量显示错误」时发现：**GitHub 上的 `main` 不是生产的真源。**

## 观察到的事实

比对服务器 `/opt/blog_li` 与 `origin/main` 上所有非迁移、非测试的 Python 文件（各 66 个）：

| 项 | 结果 |
| --- | --- |
| 仓库有、服务器无 | 0 个 |
| 两边都有、内容不同 | 2 个 |

两个差异文件：

| 文件 | 差异 | 哪边更新 |
| --- | --- | --- |
| `apps/upload/views.py` | 服务器把 `validate_file_type` / `validate_file_size` 抽到了 `apps/upload/validation.py`，并使用 `apps/user/permissions.py` 的 `IsContentEditor`、`StorageCapacityReached` | **服务器更新**（这些模块在仓库里不存在） |
| `apps/comment/serializers.py` | 内容不同 | 有一处改动部署了但没入库 |

服务器上另外多出的 80 个文件全部是 macOS 元数据垃圾（`._xxx.py`、`.___init__.py`），不是代码，可以随时清理。

结论：生产代码是仓库 `main` 的**超集**，包含若干未提交的改动。`main` 直接部署到服务器会造成**功能降级**。

## 这意味着什么

- 「push 到 main 就是发布」目前对 `blog_li` **不成立**：服务器不是 git 仓库，代码是手工拷贝上去的。
- 任何基于 `main` 的全量部署都会回退服务器上更新的代码。
- 影响范围不只是这两个文件：缺失的 `validation.py`、`permissions.py` 说明还有配套模块只存在于服务器。

## 建议的收敛步骤

1. **以服务器为准**，把缺的东西补回仓库（不要反过来）：
   - 从服务器取回 `apps/upload/validation.py`、`apps/user/permissions.py`、`apps/comment/serializers.py`、`apps/upload/views.py`
   - 与 `main` 做一次人工合并，确认没有把更新覆盖成旧版
2. 在服务器上 `git init` + 关联远端，或把 `/opt/blog_li` 换成干净的 `git clone`，让「服务器代码」始终可追溯到某个 commit。
3. 参照 AstraStoreXion 的做法加一条发布流水线（构建 → scp → 原子替换 → 健康检查 → 失败回滚），让 `blog_li` 也变成 push 即发布。
4. 清理服务器上的 macOS 元数据垃圾：

   ```bash
   cd /opt/blog_li && find . -name '._*' -delete
   ```

## 本次改动的落点

`GET /api/upload/files/summary/` 已加入 `main`（commit `057351d`），并且**已单独部署到服务器**（服务器版 `views.py` 也加上了同名方法，线上返回 `{"total": 21, "totalBytes": 36092030}`）。因此该接口与前端修复目前是生效的。

但请注意：在上面的漂移收敛完成之前，**不要用 `main` 全量覆盖服务器代码**。

# Event Hunter Frontend

Event Hunter 的 React 前端，與 Go backend 分離部署，使用 TypeScript strict 與 Vite。Runtime 與
初始版本線以 [`../contracts/platform/toolchain-policy.yaml`](../contracts/platform/toolchain-policy.yaml)
為準；使用 Node 24 LTS，不使用已 EOL 的 Node 25。

## 技術棧

- React 19 + TypeScript strict
- Vite
- React Router 7
- TanStack Query 5
- `openapi-typescript` + `openapi-fetch`
- 專案維護的 `styles.css`
- Vitest + React Testing Library + jsdom
- Karate UI scenarios 位於 `../e2e/frontend`

目前沒有安裝 Zod、React Hook Form、Tailwind、shadcn/ui、TanStack Table、virtualizer 或 MSW。
新增套件前應先確認既有 React／TypeScript／CSS 是否無法合理解決需求。

## 目前目錄

```text
src/
├── main.tsx                       # application shell、routes 與目前頁面組裝
├── api.ts                         # Event Hunter openapi-fetch client
├── scenario-api.ts                # Scenario Lab client
├── feature-guide.ts               # Guide 內容與 feature metadata
├── investigation-list-query.ts    # 案件 URL query parsing／serialization
├── observability-links.ts         # 可信 Grafana／Tempo／Loki deep links
├── generated/
│   ├── openapi.ts                 # 由根目錄 openapi.yaml 產生
│   └── event-lab-openapi.ts       # 由 Event Lab OpenAPI 產生
├── styles.css                     # 全域與 responsive UI styles
├── *.test.ts(x)                   # unit／component tests
└── test/setup.ts                  # Vitest DOM setup
```

目前頁面仍集中於 `main.tsx`。這是現況，不是長期理想；當 feature 邊界持續成長時，應在不改變
route／API 行為的前提下逐步抽成 route 與 feature modules。

## 狀態規則

- 後端資料統一由 TanStack Query 管理。
- `/api/v1/auth/me` 也是 server state；不要把角色或權限複製到另一個 global store。
- Timeline 查詢條件放在 URL search params，讓頁面可分享與重新整理後恢復。
- Modal、active tab、sidebar 等短期狀態使用 React local state。
- 不用 React Context 或 Redux 儲存 server state。
- React Router loader 不與 TanStack Query 重複載入同一份 API 資料。

## API 契約

根目錄的 [`../openapi.yaml`](../openapi.yaml) 是 API contract。`pnpm api:generate` 會更新
`src/generated/openapi.ts`，`pnpm api:check` 會檢查 committed generated types 是否漂移；
`src/api.ts` 的 `openapi-fetch` client 直接使用 generated `paths`，不得手寫另一份 API response interface。
Scenario Lab 對應 `../contracts/event-lab/event-lab.openapi.yaml` 與 `scenario-api:*` scripts。

MVP `/login` 直接顯示 Viewer／Investigator／Admin 三張角色卡，呼叫 Demo Session API 後由瀏覽器
保存 HttpOnly Cookie；前端不能讀取或自行偽造 Cookie。route guard 依 `/auth/me` 的 permissions
決定可見操作，後端仍必須再次授權。正式環境會替換為 OIDC，不在 MVP 建立帳號密碼流程。

## 測試與品質指令

```text
pnpm dev
pnpm api:generate
pnpm api:check
pnpm scenario-api:check
pnpm typecheck
pnpm test:run
pnpm build
```

Compose 會以 Nginx 提供 production build，並將同源 `/api/*` 轉發到 Go API，保留 HttpOnly
session cookie；因此瀏覽器使用 `http://localhost:28334/login` 時不需要額外 CORS 設定。
`VITE_GRAFANA_URL` 是唯一允許產生 outbound Grafana／Tempo／Loki deep link 的基底；前端只接受
不含 credentials、query 或 fragment 的 HTTP(S) URL，事件識別碼與查詢內容會另外安全編碼。

瀏覽器業務流程使用 `../e2e/frontend` 的 Karate feature file；元件、query parser、API wrapper 與
deep-link builder 使用 Vitest／React Testing Library，並以 stubbed `fetch` 隔離 HTTP。

## 實作限制

- 不在前端重做 Grafana Logs／Metrics／Traces Console。
- 不把 access token 放入 `localStorage`。
- Mutation request 使用 same-origin、`credentials: include` 與後端 CSRF／Origin 防護；不要只靠隱藏按鈕做授權。
- 不使用 `dangerouslySetInnerHTML` 顯示事件 payload。
- Timeline 查詢必須有界，並支援取消請求、partial result、empty state 與 retry。

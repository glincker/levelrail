import * as vscode from "vscode";
import { Api, AppRow } from "./api";

const TOKEN_KEY = "levelrail.token";

class AppTree implements vscode.TreeDataProvider<AppRow> {
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;
  private rows: AppRow[] = [];

  constructor(private readonly apiFor: () => Promise<Api | undefined>) {}

  async refresh(): Promise<void> {
    const api = await this.apiFor();
    this.rows = api ? await api.listApps() : [];
    this.changed.fire();
  }

  getTreeItem(row: AppRow): vscode.TreeItem {
    return new vscode.TreeItem(row.name);
  }

  getChildren(): AppRow[] {
    return this.rows;
  }
}

export function activate(ctx: vscode.ExtensionContext): void {
  const apiFor = async (): Promise<Api | undefined> => {
    const url = vscode.workspace.getConfiguration("levelrail").get<string>("apiUrl", "");
    const token = await ctx.secrets.get(TOKEN_KEY);
    if (!url || !token) {
      void vscode.window.showWarningMessage("Set levelrail.apiUrl and run Levelrail: Set API token.");
      return undefined;
    }
    return new Api(url, token);
  };
  const tree = new AppTree(apiFor);
  const out = vscode.window.createOutputChannel("Levelrail");

  ctx.subscriptions.push(
    vscode.window.registerTreeDataProvider("levelrail.apps", tree),
    vscode.commands.registerCommand("levelrail.setToken", async () => {
      const token = await vscode.window.showInputBox({ prompt: "Levelrail API token", password: true });
      if (token) await ctx.secrets.store(TOKEN_KEY, token);
    }),
    vscode.commands.registerCommand("levelrail.refresh", () => tree.refresh()),
    vscode.commands.registerCommand("levelrail.deploy", async (row?: AppRow) => {
      const api = await apiFor();
      if (!api) return;
      const app = row?.name ?? (await vscode.window.showInputBox({ prompt: "App name" }));
      const image = app ? await vscode.window.showInputBox({ prompt: "Image reference" }) : undefined;
      if (!app || !image) return;
      await api.deploy(app, image);
      void vscode.window.showInformationMessage(`Deploy of ${app} started.`);
    }),
    vscode.commands.registerCommand("levelrail.tailLogs", async (row?: AppRow) => {
      const api = await apiFor();
      if (!api) return;
      const app = row?.name ?? (await vscode.window.showInputBox({ prompt: "App name" }));
      const id = app ? await vscode.window.showInputBox({ prompt: "Deploy ID" }) : undefined;
      if (!app || !id) return;
      out.show(true);
      const abort = new AbortController();
      ctx.subscriptions.push({ dispose: () => abort.abort() });
      await api.tailDeployLog(app, id, (l) => out.appendLine(l), abort.signal);
    }),
  );
}

export function deactivate(): void {}

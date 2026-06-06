import * as path from 'path';
import * as fs from 'fs';
import { spawn } from 'child_process';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  StreamInfo,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;
let output: vscode.OutputChannel;

type EvaluationResult = {
  ok: boolean;
  path?: string;
  result?: string;
  output?: string;
  error?: string;
  durationMs: number;
  metrics?: Record<string, unknown>;
  diagnostics?: string[];
};

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel('SPL');
  context.subscriptions.push(output);

  context.subscriptions.push(
    vscode.commands.registerCommand('spl.runFile', runCurrentFile),
    vscode.commands.registerCommand('spl.evaluateSelection', evaluateSelection),
    vscode.commands.registerCommand('spl.sessionCheckpoint', sessionCheckpoint),
    vscode.commands.registerCommand('spl.sessionRestore', sessionRestore),
    vscode.commands.registerCommand('spl.sessionInspect', sessionInspect),
    vscode.commands.registerCommand('spl.toolsFfmpegStatus', () => runSpltoolTask(context, 'Tools FFmpeg Status', ['media', 'ffmpeg-status'])),
    vscode.commands.registerCommand('spl.toolsInstallFfmpeg', () => runSpltoolTask(context, 'Tools Install FFmpeg', ['media', 'install-ffmpeg', '--apply'])),
    vscode.commands.registerCommand('spl.toolsPreviewBulkRename', () => previewBulkRename(context)),
    vscode.commands.registerCommand('spl.insertNativeOSExample', insertNativeOSExample),
    vscode.commands.registerCommand('spl.restartLanguageServer', async () => {
      await restartLanguageServer(context);
    }),
    vscode.commands.registerCommand('spl.showOutput', () => output.show())
  );

  await ensureLanguageServer(context);
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}

async function startLanguageServer(context: vscode.ExtensionContext): Promise<void> {
  if (client) {
    return;
  }
  const serverOptions: ServerOptions = () => {
    const terminalOptions = resolveServerCommand(context);
    output.appendLine(`Starting SPL language server: ${terminalOptions.command} ${terminalOptions.args.join(' ')}`);
    const child = spawn(terminalOptions.command, terminalOptions.args, {
      cwd: terminalOptions.cwd,
      shell: false,
      stdio: 'pipe',
    });
    child.stderr.on('data', (chunk: Buffer) => output.append(chunk.toString()));
    return Promise.resolve({ reader: child.stdout, writer: child.stdin } satisfies StreamInfo);
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'spl' }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.spl'),
    },
    outputChannel: output,
  };

  client = new LanguageClient('splLanguageServer', 'SPL Language Server', serverOptions, clientOptions);
  context.subscriptions.push(client);
  await client.start();
}

async function restartLanguageServer(context: vscode.ExtensionContext): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
  await ensureLanguageServer(context);
  if (client) {
    vscode.window.setStatusBarMessage('SPL language server restarted', 2500);
  }
}

async function ensureLanguageServer(context: vscode.ExtensionContext): Promise<void> {
  if (client) {
    return;
  }
  try {
    await startLanguageServer(context);
  } catch (err) {
    client = undefined;
    const message = err instanceof Error ? err.message : String(err);
    output.appendLine(`Failed to start SPL language server: ${message}`);
    void vscode.window.showWarningMessage('SPL language server failed to start. Run "SPL: Show Output" for details.');
  }
}

function resolveServerCommand(context: vscode.ExtensionContext): { command: string; args: string[]; cwd: string } {
  const config = vscode.workspace.getConfiguration('spl');
  const toolPath = config.get<string>('toolPath', '').trim();
  const mode = config.get<string>('serverMode', 'auto');
  const workspaceRoot = resolveRepositoryRoot(context);

  if ((mode === 'toolPath' || mode === 'auto') && toolPath) {
    return { command: toolPath, args: ['lsp', '--stdio'], cwd: workspaceRoot };
  }

  if (mode === 'toolPath' && !toolPath) {
    output.appendLine('spl.serverMode is toolPath but spl.toolPath is empty; falling back to go run.');
  }

  return { command: 'go', args: ['run', './cmd/spltool', 'lsp', '--stdio'], cwd: workspaceRoot };
}

function resolveRepositoryRoot(context: vscode.ExtensionContext): string {
  for (const folder of vscode.workspace.workspaceFolders ?? []) {
    const candidate = path.join(folder.uri.fsPath, 'cmd', 'spltool');
    if (fs.existsSync(candidate)) {
      return folder.uri.fsPath;
    }
  }
  const parent = path.resolve(context.extensionPath, '..');
  if (fs.existsSync(path.join(parent, 'cmd', 'spltool'))) {
    return parent;
  }
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? process.cwd();
}

async function runCurrentFile(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor || !client) {
    return;
  }
  await editor.document.save();
  const result = await requestEvaluation(editor.document.uri, editor.document.getText());
  showEvaluation('Run Current File', result);
}

async function evaluateSelection(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor || !client) {
    return;
  }
  const selectionText = editor.selection.isEmpty ? editor.document.getText() : editor.document.getText(editor.selection);
  const result = await requestEvaluation(editor.document.uri, selectionText);
  showEvaluation('Evaluate Selection', result);
}

function activeSPLEditor(): vscode.TextEditor | undefined {
  const editor = vscode.window.activeTextEditor;
  if (!editor || editor.document.languageId !== 'spl') {
    vscode.window.showWarningMessage('Open an SPL file first.');
    return undefined;
  }
  return editor;
}

async function requestEvaluation(uri: vscode.Uri, text: string): Promise<EvaluationResult> {
  if (!client) {
    throw new Error('SPL language server is not running');
  }
  const config = vscode.workspace.getConfiguration('spl');
  return client.sendRequest<EvaluationResult>('spl/evaluate', {
    uri: uri.toString(),
    text,
    options: {
      profile: config.get<string>('evaluation.profile', 'untrusted'),
      timeoutMs: config.get<number>('evaluation.timeoutMs', 1500),
      maxOutputBytes: config.get<number>('evaluation.maxOutputBytes', 65536),
      maxExecOutputBytes: config.get<number>('evaluation.maxExecOutputBytes', 65536),
      allowedCapabilities: normalizedStringList(config.get<string[]>('evaluation.allowedCapabilities', [])),
      allowedExecCommands: normalizedStringList(config.get<string[]>('evaluation.allowedExecCommands', [])),
      allowedNativeModules: normalizedStringList(config.get<string[]>('evaluation.allowedNativeModules', ['native/os'])),
      deniedNativeModules: normalizedStringList(config.get<string[]>('evaluation.deniedNativeModules', [])),
      allowedFileReadPaths: workspaceFolderPaths(uri),
    },
  });
}

function showEvaluation(label: string, result: EvaluationResult): void {
  output.show(true);
  output.appendLine('');
  output.appendLine(`[${label}] ${result.ok ? 'OK' : 'ERROR'} (${result.durationMs}ms)`);
  if (result.path) {
    output.appendLine(path.basename(result.path));
  }
  if (result.output) {
    output.appendLine('Output:');
    output.appendLine(result.output.trimEnd());
  }
  if (result.result) {
    output.appendLine(`Result: ${result.result}`);
  }
  if (result.error) {
    output.appendLine('Error:');
    output.appendLine(result.error);
  }
  if (result.metrics) {
    output.appendLine(`Metrics: ${JSON.stringify(result.metrics)}`);
  }
  if (result.diagnostics?.length) {
    output.appendLine('Diagnostics:');
    output.appendLine(result.diagnostics.join('\n'));
  }
}

async function sessionCheckpoint(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor || !client) {
    return;
  }
  const name = await vscode.window.showInputBox({ prompt: 'Checkpoint name', value: 'manual' });
  if (!name) {
    return;
  }
  const result = await client.sendRequest<Record<string, unknown>>('spl/sessionCheckpoint', {
    uri: editor.document.uri.toString(),
    name,
  });
  output.show(true);
  output.appendLine(`[Session Checkpoint] ${JSON.stringify(result)}`);
}

async function sessionRestore(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor || !client) {
    return;
  }
  const name = await vscode.window.showInputBox({ prompt: 'Checkpoint name to restore', value: 'manual' });
  if (!name) {
    return;
  }
  const result = await client.sendRequest<Record<string, unknown>>('spl/sessionRestore', {
    uri: editor.document.uri.toString(),
    name,
  });
  output.show(true);
  output.appendLine(`[Session Restore] ${JSON.stringify(result)}`);
}

async function sessionInspect(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor || !client) {
    return;
  }
  const result = await client.sendRequest<Record<string, unknown>>('spl/sessionInspect', {
    uri: editor.document.uri.toString(),
  });
  output.show(true);
  output.appendLine('[Session Inspect]');
  output.appendLine(JSON.stringify(result, null, 2));
}

async function previewBulkRename(context: vscode.ExtensionContext): Promise<void> {
  const folder = await vscode.window.showOpenDialog({
    canSelectFiles: false,
    canSelectFolders: true,
    canSelectMany: false,
    title: 'Select folder to preview bulk rename',
  });
  if (!folder?.length) {
    return;
  }
  const match = await vscode.window.showInputBox({ prompt: 'Glob match', value: '*.jpg' });
  if (!match) {
    return;
  }
  const template = await vscode.window.showInputBox({ prompt: 'Rename template', value: '{date}_{seq}.{ext}' });
  if (!template) {
    return;
  }
  await runSpltoolTask(context, 'Tools Preview Bulk Rename', ['files', 'rename', folder[0].fsPath, '--match', match, '--template', template, '--json']);
}

async function insertNativeOSExample(): Promise<void> {
  const editor = activeSPLEditor();
  if (!editor) {
    return;
  }
  const snippet = new vscode.SnippetString([
    'import "native/os" as os;',
    '',
    'let result = os.run("${1:go}", [${2:"version"}], {',
    '  "timeout_ms": ${3:1500},',
    '  "max_output_bytes": ${4:65536}',
    '});',
    '',
    'if (result["ok"]) {',
    '  print result["stdout"];',
    '} else {',
    '  print result["stderr"];',
    '  print result["error"];',
    '}',
    '',
  ].join('\n'));
  await editor.insertSnippet(snippet);
}

function normalizedStringList(values: readonly string[] | undefined): string[] {
  return Array.from(new Set((values ?? []).map((value) => value.trim()).filter(Boolean)));
}

function workspaceFolderPaths(uri: vscode.Uri): string[] {
  const folder = vscode.workspace.getWorkspaceFolder(uri);
  return folder ? [folder.uri.fsPath] : [];
}

async function runSpltoolTask(context: vscode.ExtensionContext, label: string, args: string[]): Promise<void> {
  const terminalOptions = resolveSpltoolCommand(context, args);
  output.show(true);
  output.appendLine('');
  output.appendLine(`[${label}] ${terminalOptions.command} ${terminalOptions.args.join(' ')}`);
  await new Promise<void>((resolve) => {
    const child = spawn(terminalOptions.command, terminalOptions.args, {
      cwd: terminalOptions.cwd,
      shell: false,
      stdio: 'pipe',
    });
    child.stdout.on('data', (chunk: Buffer) => output.append(chunk.toString()));
    child.stderr.on('data', (chunk: Buffer) => output.append(chunk.toString()));
    child.on('error', (err) => {
      output.appendLine(`Failed: ${err.message}`);
      void vscode.window.showErrorMessage(`${label} failed. Run "SPL: Show Output" for details.`);
      resolve();
    });
    child.on('close', (code) => {
      output.appendLine(`[${label}] exited with code ${code ?? 'unknown'}`);
      if (code === 0) {
        vscode.window.setStatusBarMessage(`${label} complete`, 2500);
      } else {
        void vscode.window.showWarningMessage(`${label} exited with code ${code}. Run "SPL: Show Output" for details.`);
      }
      resolve();
    });
  });
}

function resolveSpltoolCommand(context: vscode.ExtensionContext, args: string[]): { command: string; args: string[]; cwd: string } {
  const config = vscode.workspace.getConfiguration('spl');
  const toolPath = config.get<string>('toolPath', '').trim();
  const workspaceRoot = resolveRepositoryRoot(context);
  if (toolPath) {
    return { command: toolPath, args, cwd: workspaceRoot };
  }
  return { command: 'go', args: ['run', './cmd/spltool', ...args], cwd: workspaceRoot };
}

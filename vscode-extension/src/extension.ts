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
};

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel('SPL');
  context.subscriptions.push(output);

  await startLanguageServer(context);

  context.subscriptions.push(
    vscode.commands.registerCommand('spl.runFile', runCurrentFile),
    vscode.commands.registerCommand('spl.evaluateSelection', evaluateSelection),
    vscode.commands.registerCommand('spl.restartLanguageServer', async () => {
      await restartLanguageServer(context);
    }),
    vscode.commands.registerCommand('spl.showOutput', () => output.show())
  );
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}

async function startLanguageServer(context: vscode.ExtensionContext): Promise<void> {
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
  await startLanguageServer(context);
  vscode.window.setStatusBarMessage('SPL language server restarted', 2500);
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
}

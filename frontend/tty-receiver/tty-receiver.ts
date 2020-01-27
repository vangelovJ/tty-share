import { Terminal, IEvent, IDisposable } from "xterm";
import { FitAddon } from 'xterm-addon-fit';

import base64 from './base64';

interface IRectSize {
    width: number;
    height: number;
}

class TTYReceiver {
    private xterminal: Terminal;
    private containerElement: HTMLElement;
    private fitAddon: FitAddon;
    private connection: WebSocket;
    private retry: boolean;

    constructor(wsAddress: string, container: HTMLDivElement) {
        this.xterminal = new Terminal({
            cursorBlink: true,
            macOptionIsMeta: true,
            scrollback: 1000,
            fontSize: 16,
            letterSpacing: 0,
        });
        this.retry = true;
        this.fitAddon = new FitAddon();
        this.xterminal.loadAddon(this.fitAddon);
        this.containerElement = container;
        this.xterminal.open(container);
        var ttyReceiver = this;
        this.xterminal.onData(function (data: string) {
            let writeMessage = {
                Type: "Write",
                Data: base64.encode(JSON.stringify({ Size: data.length, Data: base64.encode(data)})),
            }
            let dataToSend = JSON.stringify(writeMessage);
            ttyReceiver.connection.send(dataToSend);
        });

        this.xterminal.onResize((e) => {
            let writeMessage = {
                Type: "WinSize",
                Data: base64.encode(JSON.stringify({ Cols: e.cols, Rows: e.rows})),
            }
            let dataToSend = JSON.stringify(writeMessage)
            ttyReceiver.connection.send(dataToSend);
        })
        window.onresize = () => {
            ttyReceiver.fitAddon.fit();
        }
        this.xterminal.write("Connecting to the server...\n\r");
        this.initWebSocket(wsAddress)
    }

    private initWebSocket(wsAddress: string) {
        this.connection = new WebSocket(wsAddress);
        var ttyReceiver = this;
        this.connection.onopen = (evt: Event) => {
            this.xterminal.focus();
            this.xterminal.resize(this.xterminal.cols-1, this.xterminal.rows-1);
            this.fitAddon.fit();
            this.xterminal.setOption('cursorBlink', true);
        }
        this.connection.onclose =  (evt: CloseEvent) => {
            this.xterminal.blur();
            this.xterminal.setOption('cursorBlink', false);
            this.xterminal.write('Session closed\n\r');
            if (ttyReceiver.retry) {
                this.xterminal.write('Reconnecting after 3 second...\n\r');
                setTimeout(() => {
                    this.initWebSocket(wsAddress)
                }, 3000);
            }
        }
        this.connection.onmessage = (ev: MessageEvent) => {
            let message = JSON.parse(ev.data)
            if (message.Type === "Write") {
                let msgData = base64.decode(message.Data)
                let writeMsg = JSON.parse(msgData)
                this.xterminal.writeUtf8(base64.base64ToArrayBuffer(writeMsg.Data));
            }
            if (message.Type === "Terminate") {
                ttyReceiver.retry = false;
            }
        }
    }

    // Get the pixels size of the element, after all CSS was applied. This will be used in an ugly
    // hack to guess what fontSize to set on the xterm object. Horrible hack, but I feel less bad
    // about it seeing that VSV does it too:
    // https://github.com/microsoft/vscode/blob/d14ee7613fcead91c5c3c2bddbf288c0462be876/src/vs/workbench/parts/terminal/electron-browser/terminalInstance.ts#L363
    private getElementPixelsSize(element: HTMLElement): IRectSize {
        const defView = this.containerElement.ownerDocument.defaultView;
        let width = parseInt(defView.getComputedStyle(element).getPropertyValue('width').replace('px', ''), 10);
        let height = parseInt(defView.getComputedStyle(element).getPropertyValue('height').replace('px', ''), 10);

        return {
            width,
            height,
        }
    }

    // Tries to guess the new font size, for the new terminal size, so that the rendered terminal
    // will have the newWidth and newHeight dimensions
    private guessNewFontSize(newCols: number, newRows: number, targetWidth: number, targetHeight: number): number {
        const cols = this.xterminal.cols;
        const rows = this.xterminal.rows;
        const fontSize = this.xterminal.getOption('fontSize');
        const xtermPixelsSize = this.getElementPixelsSize(this.containerElement.querySelector(".xterm-screen"));

        const newHFontSizeMultiplier =  (cols / newCols) * (targetWidth / xtermPixelsSize.width);
        const newVFontSizeMultiplier = (rows / newRows) * (targetHeight / xtermPixelsSize.height);

        let newFontSize;

        if (newHFontSizeMultiplier > newVFontSizeMultiplier) {
            newFontSize = Math.floor(fontSize * newVFontSizeMultiplier);
        } else {
            newFontSize = Math.floor(fontSize * newHFontSizeMultiplier);
        }
        return newFontSize;
    }
}

export {
    TTYReceiver
}

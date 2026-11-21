const colorLogo = app.pb.buildURL("/_/extensions/aist/logo.svg");
const bwLogo = app.pb.buildURL("/_/extensions/aist/logo-bw.svg");

app.store.mainLogo = bwLogo;
app.store.headerLogo = bwLogo;

const colorScheme = window.matchMedia("(prefers-color-scheme: dark)");
const syncFavicon = () => {
    app.store.favicon = colorScheme.matches ? bwLogo : colorLogo;
};
syncFavicon();
colorScheme.addEventListener("change", syncFavicon);

const style = document.createElement("style");
style.textContent = [
    ".main-logo { max-height: 96px; }",
    ".app-header .logo img { max-height: 28px; }",
    // Keep multi-line audit values (tool results, file contents) readable in log details.
    ".log-preview-modal td .txt { white-space: pre-wrap; word-break: break-all; }",
].join(" ");
document.head.appendChild(style);

document.title = "AIST";

const brandPhrases = [
    ["Setup your PocketBase instance", "Setup your AIST instance"],
    ["Backup and restore your PocketBase data", "Backup and restore your AIST data"],
];

function rebrandText() {
    const walker = document.createTreeWalker(document.documentElement, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
        const node = walker.currentNode;
        const tag = node.parentElement?.tagName;
        if (tag === "SCRIPT" || tag === "STYLE") continue;
        for (const [from, to] of brandPhrases) {
            if (node.nodeValue.includes(from)) {
                node.nodeValue = node.nodeValue.replaceAll(from, to);
            }
        }
    }
}

let pending = false;
function scheduleRebrand() {
    if (pending) return;
    pending = true;
    requestAnimationFrame(() => {
        pending = false;
        rebrandText();
    });
}

scheduleRebrand();
new MutationObserver(scheduleRebrand).observe(document.documentElement, {
    childList: true,
    subtree: true,
    characterData: true,
});

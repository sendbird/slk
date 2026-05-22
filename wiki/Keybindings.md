# Keybindings

| Key | Mode | Action |
|---|---|---|
| `j` / `k` | Normal | Move down/up in channel list or messages |
| `h` / `l` | Normal | Switch focus between panels |
| `Tab` / `Shift+Tab` | Normal | Cycle focus |
| `Enter` | Normal (sidebar) | Open selected channel, or toggle a section header |
| `Space` | Normal (sidebar) | Toggle the selected section header (collapse/expand) |
| `Enter` | Normal (message) | Open thread |
| `i` / `F2` | Normal | Enter insert mode (`F2` is useful when a CJK IME consumes alphabetic keys for composition) |
| `Esc` | Insert / Command | Return to normal mode |
| `Enter` / `Ctrl+Enter` | Insert | Send message (`Ctrl+Enter` is useful when IME composition consumes plain Enter) |
| `Shift+Enter` | Insert | Newline |
| `Ctrl+V` | Insert | Smart paste — image / file path / text (use `Ctrl+V`, not the terminal's `Ctrl+Shift+V`) |
| `Ctrl+U` | Insert | Clear compose (text + pending attachments) |
| `Ctrl+U` / `Ctrl+D` | Normal | Half-page up / down |
| `Up` | Insert | Previous line; on the first line, jump to start of message |
| `Down` | Insert | Next line; on the last line, jump to end of message |
| `gg` / `G` | Normal | Jump to top / bottom |
| `Ctrl+b` | Any | Toggle sidebar |
| `Ctrl+]` | Any | Toggle thread panel |
| `Ctrl+t` / `Ctrl+p` | Any | Fuzzy channel finder |
| `Ctrl+w` | Any | Workspace picker |
| `1`–`9` | Normal | Jump to workspace N |
| `r` | Normal (message) | Open reaction picker |
| `R` | Normal (message) | Quick-toggle existing reactions |
| `E` | Normal (message) | Edit your own message |
| `D` | Normal (message) | Delete your own message (with confirmation) |
| `U` | Normal (message) | Mark selected message and everything newer as unread |
| `Y` / `C` | Normal (message) | Copy message permalink |
| `O` / `v` | Normal (message) | Open full-screen image preview |
| `Esc` / `q` | Preview | Close preview |
| `Enter` | Preview | Open in system image viewer |
| `h` / `←` | Preview | Previous image (when message has multiple) |
| `l` / `→` | Preview | Next image (when message has multiple) |
| Click | Any (on image) | Open full-screen preview |
| `Ctrl+y` | Any | Switch theme |
| `Ctrl+s` | Any | Set status (Active / Away / DND snooze) |
| `q` | Normal | Quit (with confirmation) |
| `Q` | Normal | Quit immediately |
| `Ctrl+c` | Any | Quit (with confirmation) |

Custom keybinding overrides are on the roadmap — see [[Tradeoffs and Non-Goals|Tradeoffs-and-Non-Goals]].


## Korean / CJK IME notes

Terminal IME composition is owned by the OS and terminal emulator before input reaches `slk`. With Korean IME active, the first physical `i` may appear as pending `ㅑ` and may not be delivered to `slk` until the IME commits it. `slk` maps committed Korean jamo such as `ㅑ` back to QWERTY shortcuts when those events reach the app, and uses terminal-provided base-key metadata when available, but it cannot force the terminal to send keys that the IME is still composing.


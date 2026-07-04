# Starting pseudosonic-go automatically on device mount

This, naturally, only works in Linux. Other OSes have their own mechanisms to run things on device insertion or, well, don't.

The example `.path` and `.service` files herein go into `$HOME/.config/systemd/user`. There are four of them to solve a deficiency in `systemd`, if there wasn't one, you'd be able to get away with at most two. They assume that the card in your player has the filesystem label `ECHO_SD` and that when mounted normally through your desktop, it appears at `/media/$USER/ECHO_SD`.

The `sync-player` is the very basic example script that does the actual favorites synchronization.

Install the systemd scripts, `enable` the `.path` units and `start` them, and you should be good.


# Four2Six

A tool to forward IPv4 traffic to an IPv6 destination. It can update its destination with a webhook.

I've built this tool to solve a very specific problem. My ISP does not provide me with a dual stack internet connection so the only way to access my home network from the internet is by using IPv6. This tool runs on a cloud server and listens on every IPv4 request to my home network and forwards it via IPv6.

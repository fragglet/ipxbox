
#ifndef DBIPX_H
#define DBIPX_H

#ifdef __cplusplus
extern "C" {
#endif

void DBIPX_Connect(const char *addr, int port);
void DBIPX_GetAddress(char *addr);
void DBIPX_SendPacket(struct ipx_header *pkt, size_t len);
void DBIPX_Poll(void);

extern struct ipx_address dbipx_local_addr;

#ifdef __cplusplus
}
#endif

#endif /* DBIPX_H */



#ifndef DBIPX_H
#define DBIPX_H

#ifdef __cplusplus
extern "C" {
#endif

typedef void (*dbipx_packet_callback)(const struct ipx_header *pkt,
                                      size_t len);

void DBIPX_Connect(const char *addr, int port);
void DBIPX_GetAddress(char *addr);
void DBIPX_SendPacket(struct ipx_header *pkt, size_t len);
void DBIPX_SetCallback(dbipx_packet_callback callback);
void DBIPX_Poll(void);

extern struct ipx_address dbipx_local_addr;

#ifdef __cplusplus
}
#endif

#endif /* DBIPX_H */


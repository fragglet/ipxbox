
#ifndef DBIPX_H
#define DBIPX_H

#ifdef __cplusplus
extern "C" {
#endif

void DBIPX_Connect(const char *addr, int port);
void DBIPX_GetAddress(char *addr);

#ifdef __cplusplus
}
#endif

#endif /* DBIPX_H */


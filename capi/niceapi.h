#include "capi.h"

#include <stddef.h>
#include <stdbool.h>
#include <string.h>
 
typedef struct pool_login_args {
  // username: a properly formatted pool username.
  const char* username;
  // rigid: a properly formatted rig id, or null if no rigid is specified by the user.
  const char* rigid;
  // wallet: a properly formatted wallet address; can be null for username-only logins. If wallet is
  //         null, pool server will return a warning if the username has not previously been
  //         associated with a wallet.
  const char* wallet;
  // agent: a string that informs the pool server of the miner client details, e.g. name and version
  //        of the software using this API.
  const char* agent;
  // config: advanced options config string, can be null.
  const char* config;
} pool_login_args;
 
 
typedef struct pool_login_response {
  // code = 1: login successful; if message is non-null, it's a warning/info message from pool
  //           server that should be shown to the user
  //
  // code < 0: login unsuccessful; couldn't reach pool server. Caller should retry later. message
  //           will contain the connection-level error encountered.
  //
  // code > 1: login unsuccessful; pool server refused login. Message will contain information that
  //           can be shown to user to help fix the problem. Caller should retry with new login
  //           parameters.
  int code;
  const char* message; // must be freed by the caller
} pool_login_response;
 
// pool_login logs into the remote pool server with the provided login info.
pool_login_response pool_login(const pool_login_args *args) {
  struct PoolLogin_return r;
  r = PoolLogin((char*)args->username,
				args->rigid == NULL ? "" : (char*)args->rigid,
				args->wallet == NULL ? "" : (char*)args->wallet,
				(char*)args->agent,
				args->config == NULL ? "" : (char*)args->config);
  pool_login_response response;
  response.code = (int)r.r0;
  if (strlen(r.r1) > 0) {
	response.message = r.r1;
  } else {
	response.message = NULL;
  }
  return response;
}

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


typedef struct start_miner_args {
  // threads specifies the initial # of threads to mine with. Must be >=1
  int threads;

  // begin/end hours (24 time) of the time during the day where mining should be paused. Set both
  // to 0 if there is no excluded range.
  int exclude_hour_start;
  int exclude_hour_end;
} start_miner_args;

typedef struct start_miner_response {
  // code == 1: miner started successfully.
  //
  // code == 2: miner started successfully but hugepages could not be enabled, so mining may be
  //            slow. You can suggest to the user that a machine restart might help resolve this.
  //
  // code > 2: miner failed to start due to bad config, see details in message. For example, an
  //           invalid number of threads or invalid hour range may have been specified.
  //
  // code < 0: non-recoverable error, message will provide details. program should exit after
  //           showing message.
  int code;
  const char* message; // must be freed by the caller
} start_miner_response;

// call only after successful pool_login. This should only be called once!
start_miner_response start_miner(const start_miner_args *args) {
  struct StartMiner_return r =
    StartMiner((GoInt)args->threads, (GoInt)args->exclude_hour_start, (GoInt)args->exclude_hour_end);
  start_miner_response response;
  response.code = (int)r.r0;
  if (strlen(r.r1) > 0) {
	response.message = r.r1;
  } else {
	response.message = NULL;
  }
  return response;
}

#!/bin/bash
#
# Copyright (C) 2015 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#         http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

KUBE_ROOT=$(dirname "${BASH_SOURCE}")/../..

if [[ ! -z "${1:-}" ]]; then
  export KUBE_GCE_ZONE="${1}"
fi
if [[ ! -z "${2:-}" ]]; then
  export MULTIZONE="${2}"
fi
if [[ ! -z "${3:-}" ]]; then
  export KUBE_REPLICATE_EXISTING_MASTER="${3}"
fi
if [[ ! -z "${4:-}" ]]; then
  export KUBE_USE_EXISTING_MASTER="${4}"
fi
if [[ -z "${NUM_NODES:-}" ]]; then
  export NUM_NODES=3
fi

source "${KUBE_ROOT}/hack/e2e-internal/e2e-up.sh"

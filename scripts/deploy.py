#!/usr/bin/env python3

"""
Creates a new run in the Terraform Cloud workspace
"""

import os
import tarfile
import logging
from argparse import ArgumentParser
from glob import glob
from io import BytesIO
from typing import Tuple, Optional, List

import requests

TF_API_HOST = os.getenv("TF_API_HOST", "https://app.terraform.io/api/v2")

TOKEN = os.getenv("TF_USER_TOKEN")
WORKSPACE = os.getenv("TF_WORKSPACE_NAME")
ORG = os.getenv("TF_ORG_NAME")

DEFAULT_GLOBS = ["*.tf", "*.auto.tfvars"]

HEADERS = {
    "Authorization": f"Bearer {TOKEN}",
    "Content-Type": "application/vnd.api+json",
}


class MissingEnvironmentError(Exception):
    pass


class BadConfigVersionError(Exception):
    pass


def fetch_workspace_id() -> str:
    print("Fetching workspace ID")
    resp = requests.get(
        f"{TF_API_HOST}/organizations/{ORG}/workspaces/{WORKSPACE}", headers=HEADERS
    )
    resp.raise_for_status()

    workspace_id = resp.json()["data"]["id"]
    print("Workspace ID:", workspace_id)

    return workspace_id


def create_config_version(workspace_id: str) -> Tuple[str, str]:
    print("Creating new config version")
    resp = requests.post(
        f"{TF_API_HOST}/workspaces/{workspace_id}/configuration-versions",
        headers=HEADERS,
        json={
            "data": {
                "type": "configuration-version",
                "attributes": {"auto-queue-runs": False},
            }
        },
    )
    resp.raise_for_status()
    data = resp.json()["data"]
    print("Version ID:", data["id"])
    return data["id"], data["attributes"]["upload-url"]


def archive_repo(root: str, globs: List[str], files: List[str]) -> BytesIO:
    print("Creating archive")

    if root != ".":
        prefix = root
    root = os.path.abspath(root)
    out = BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz") as tar:
        for f in files:
            tar.add(os.path.join(root, f), arcname=os.path.join(prefix, f))

        for pathspec in globs:
            for tf_file in glob(os.path.join(root, pathspec)):
                tar.add(
                    tf_file, arcname=os.path.join(prefix, os.path.basename(tf_file))
                )

    out.seek(0)
    return out


def upload_archive(archive: BytesIO, upload_url: str) -> None:
    print("Uploading archive")
    resp = requests.put(
        upload_url, headers={"Content-Type": "application/octet-stream"}, data=archive
    )
    resp.raise_for_status()


def wait_for_configuration_uploaded(config_id: str) -> None:
    print("Waiting for config version to become ready")
    while True:
        resp = requests.get(
            f"{TF_API_HOST}/configuration-versions/{config_id}", headers=HEADERS
        )
        resp.raise_for_status()

        status = resp.json()["data"]["attributes"]["status"]
        if status == "uploaded":
            return
        elif status == "errored":
            error = resp.json()["data"]["attributes"]["error"]
            msg = resp.json()["data"]["attributes"]["error-message"]
            raise BadConfigVersionError(
                f"ERR: bad status on config-version upload: {error} - {msg}"
            )


def create_run(workspace_id: str, config_id: str, message: Optional[str] = None) -> str:
    print("Creating new run")

    params = {
        "data": {
            "type": "runs",
            "attributes": {"is-destroy": False, "message": message},
            "relationships": {
                "workspace": {"data": {"type": "workspaces", "id": workspace_id}},
                "configuration-version": {
                    "data": {"type": "configuration-versions", "id": config_id}
                },
            },
        }
    }
    resp = requests.post(f"{TF_API_HOST}/runs", headers=HEADERS, json=params)
    try:
        resp.raise_for_status()
    except requests.exceptions.HTTPError:
        print("PARAMS:", params)
        print("ERR:", resp.json())
        raise

    run_id = resp.json()["data"]["id"]
    print("Run ID:", run_id)
    return run_id


def env_check() -> None:
    if any(var is None for var in (TOKEN, WORKSPACE, ORG)):
        raise MissingEnvironmentError(
            "must set TF_USER_TOKEN, TF_WORKSPACE_NAME, and TF_ORG_NAME env vars"
        )


def main() -> None:
    parser = ArgumentParser(description=__doc__)
    parser.add_argument(
        "-m", "--message", metavar="STR", help="message to attach to this run in TF"
    )
    parser.add_argument(
        "-g",
        "--glob",
        metavar="GLOB",
        action="append",
        default=DEFAULT_GLOBS,
        help="glob pattern of files to include in the run, can specify multiple times. default: [*.tf, *.auto.tfvars]",
    )
    parser.add_argument(
        "-f",
        "--file",
        metavar="FILE",
        action="append",
        default=[],
        help="individual files or directories to add directly to the run, relative to the root. Specify multiple times",
    )
    parser.add_argument(
        "-d",
        "--debug",
        action="store_true",
        default=False,
        help="enable verbose HTTP logging",
    )
    parser.add_argument(
        "root", metavar="PATH", help="path to terraform root", nargs="?", default=".",
    )
    args = parser.parse_args()

    if args.debug:
        logging.basicConfig(level=logging.DEBUG)

    env_check()

    workspace_id = fetch_workspace_id()
    config_id, upload_url = create_config_version(workspace_id)
    upload_archive(archive_repo(args.root, args.glob, args.file), upload_url)
    wait_for_configuration_uploaded(config_id)
    create_run(workspace_id, config_id, args.message)


if __name__ == "__main__":
    main()

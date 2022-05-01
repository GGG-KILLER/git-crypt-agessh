# git-crypt-agessh

Encrypt files in git using [age](https://age-encryption.org/) and ed25519 ssh keys.

## Security First!

**I have literally no idea if this is secure, I'm not a security expert, use at your own risk**. I'm only using this to encrypt security-by-obscurity type details in [mtoohey31/infra](https://github.com/mtoohey31/infra) such as usernames, port numbers, public keys, and ip addresses, **but not** important things like passwords or private keys.

## Usage

In the repository where you want to encrypt files, run:

```sh
git-crypt-agessh init
```

Then, specify the files you want to encrypt in a `.gitattributes` file:

```
# age1b33gd26rafkrbbv7hiwroiv2890otnd2mhaseyso0uad03nv7p1vz8fpqv,9njzb5gqwv0weq0f43daw9ql9d1wwuwfifc77y9krvtofdrwll5xng59da
/secrets.nix filter=git-crypt-agessh diff=git-crypt-agessh
```

The comment line preceeding the rule which matches the file should contain a comma-separated list of age public keys, which can be converted from an ed25519 ssh public key with [Mid92/ssh-to-age](9njzb5gqwv0weq0f43daw9ql9d1wwuwfifc77y9krvtofdrwll5xng59da).

Finally, when you run `git add`, the file should be encrypted by `git-crypt-agessh`'s clean filter. If you want to test that the file has actually be encrypted before pushing it to the actual remote, consider adding an ssh remote on your local host, pushing, and checking that the file contents are seemingly random bytes by running:

```sh
# in your home directory:
git init test-remote
cd test-remote
git checkout -b tmp # since we can't be on main while pushing to it

# in the original repository
git remote add test-remote "$(whoami)@localhost:~/test-remote"
git push test-remote

# in the test-remote repository
git checkout main
```

Since this remote hasn't been initialized with `git-crypt-agessh`, the contents of your encrypted files should still be initialized.

If you want to decrypt files in this new remote (or any other where one of the corresponding ssh private keys stored at `~/.ssh/id_ed25519` on that host), run:

```sh
rm encrypted_file
git-crypt-agessh init
git restore encrypted_file
```

When you run `git restore`, the filters should now be recognized, and the restored file will be decrypted.

## Related

- [vlaci/git-agecrypt](https://github.com/vlaci/git-agecrypt)
- [AGWA/git-crypt](https://github.com/AGWA/git-crypt)

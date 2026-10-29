mod build '.justfiles/build.just'
mod test '.justfiles/test.just'
mod lint '.justfiles/lint.just'

[private]
default:
    @just --list

check: (build::all) (test::all) (lint::all)

mod demo '.justfiles/demos.just'
mod release '.justfiles/release.just'

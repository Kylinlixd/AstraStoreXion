#!/usr/bin/env python
# -*- coding: utf-8 -*-

from setuptools import setup, find_packages

with open("README.md", "r", encoding="utf-8") as fh:
    long_description = fh.read()

setup(
    name="astrastore-xion-client",
    version="1.1.1",
    author="星辰离子X团队",
    author_email="support@astrastore.com",
    description="星辰离子X分布式文件存储系统的Python客户端",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="https://github.com/astrastore/astrastore-xion",
    packages=find_packages(),
    classifiers=[
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "License :: OSI Approved :: MIT License",
        "Operating System :: OS Independent",
    ],
    python_requires=">=3.9",
    install_requires=[
        "requests>=2.25.0",
        "pyyaml>=5.4.0",
    ],
)
